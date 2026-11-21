// Package hook handles the Claude Code hook events SessionStart, UserPromptSubmit, Stop and SessionEnd: it keeps each session's notes, state
// word and queue current, answers the + commands, and starts the summarizer. A hook must never fail or swallow a user's prompt
// (invariant 4), so Run reports nothing and recovers from everything.
package hook

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/lfeagan/claude-sitrep/internal/clock"
	"github.com/lfeagan/claude-sitrep/internal/notes"
	"github.com/lfeagan/claude-sitrep/internal/queue"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
	"github.com/lfeagan/claude-sitrep/internal/transcript"
)

const (
	maxInput      = 64 << 20 // the most of an event read from standard input; a big paste is still one event
	defaultWindow = 45       // seconds between summaries
)

// Hook handles events for one state folder.
type Hook struct {
	Store     state.Store
	ConfigDir string // Claude Code's folder, for transcripts
	Getenv    func(string) string
	Clock     clock.Clock
	Out       io.Writer // where a block answer goes
	// Spawn starts `sitrep <args>` detached from the hook, which must not wait for it.
	Spawn func(args ...string) error
}

type event struct {
	SessionID      string `json:"session_id"`
	HookEventName  string `json:"hook_event_name"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	Prompt         string `json:"prompt"`
	Source         string `json:"source"`
}

// Run handles one event read from in. It never fails and never panics.
func (h Hook) Run(in io.Reader) {
	defer func() { _ = recover() }()
	var ev event
	if json.NewDecoder(io.LimitReader(in, maxInput)).Decode(&ev) != nil || !state.SessionID(ev.SessionID) {
		return // the id becomes part of a file name; the inbox is the queue of no session
	}
	ev.TranscriptPath = strings.TrimRight(ev.TranscriptPath, "\n") // the shell's command substitution
	if h.Store.Ensure() != nil {
		return
	}
	s := session{Hook: h, id: state.ID(ev.SessionID), ev: ev}
	if transcript.Valid(ev.TranscriptPath, h.ConfigDir) {
		s.tp = ev.TranscriptPath
	}
	s.run()
}

type session struct {
	Hook
	id        state.ID
	ev        event
	tp        string // the transcript, when it may be read
	obj       string // the Objective in the notes, a placeholder counted as unset
	prog, now string
}

func (s *session) notes() notes.Notes { return notes.Notes{Store: s.Store} }
func (s *session) queue() queue.Queue { return queue.Queue{Store: s.Store} }

func (s *session) setState(word string) {
	_ = s.Store.WriteFile(s.Store.Hidden(s.id, "state"), []byte(word+"\n")) // a hook never fails
}

// block answers a prompt without sending it to Claude.
func (s *session) block(reason string) {
	enc := json.NewEncoder(s.Out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}{"block", reason})
}

// placeholder says an Objective is not a real one: a dash, or a command wrapper such as the one /clear leaves.
func placeholder(obj string) bool {
	return obj == "-" || strings.Contains(obj, "<command-") || strings.Contains(obj, "&lt;command-") ||
		strings.HasPrefix(obj, "<") || strings.HasPrefix(obj, "&lt;")
}

// short says a + command may act on text: one line of at most 200 characters, not blank.
func short(s string) bool {
	return !strings.Contains(s, "\n") && utf8.RuneCountInString(s) <= 200 && strings.TrimSpace(s) != ""
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func (s *session) write(obj, prog, now string) error {
	return s.notes().Write(s.id, textsafe.Sane(obj, 0), textsafe.Sane(prog, 0), textsafe.Sane(now, 0))
}

func (s *session) run() {
	if s.ev.HookEventName == "SessionEnd" {
		s.setState("ended")
		return
	}
	cwd := ""
	if s.ev.Cwd != "" {
		cwd = textsafe.Sane(s.ev.Cwd+"\n", 300).String() // the newline is part of the bash output, and becomes a space
	}
	_ = s.Store.WriteFile(s.Store.Path(s.id, "cwd"), []byte(cwd))
	s.obj, s.prog, s.now = s.notes().Get(s.id, "Objective"), s.notes().Get(s.id, "Progress"), s.notes().Get(s.id, "Now")
	if placeholder(s.obj) {
		s.obj = ""
	}
	switch s.ev.HookEventName {
	case "UserPromptSubmit":
		s.prompt()
	case "SessionStart":
		s.start()
	case "Stop":
		s.stop()
	}
}

func (s *session) prompt() {
	prompt := strings.TrimRight(s.ev.Prompt, "\n") // the shell's command substitution
	c, _ := strconv.Atoi(state.FirstLine(s.Store.Hidden(s.id, "count")))
	_ = s.Store.WriteFile(s.Store.Hidden(s.id, "count"), []byte(strconv.Itoa(c+1)+"\n")) // for the stale-summary check
	if s.plus(prompt) {
		return
	}
	s.setState("working") // only a prompt that goes to Claude starts work
	obj := s.obj
	if obj == "" {
		obj = textsafe.Sane(transcript.FirstPrompt(s.tp), 160).String()
	}
	if obj == "" {
		obj = textsafe.Sane(prompt, 160).String()
	}
	_ = s.write(obj, orDash(s.prog), textsafe.Sane(prompt, 140).String())
}

// plus handles +pin, +, +inbox and +take, and says whether the prompt was answered here. Anything that is not a short single line, or whose
// write fails, goes on to Claude untouched.
func (s *session) plus(prompt string) bool {
	switch {
	case strings.HasPrefix(prompt, "+pin "):
		if text := strings.TrimPrefix(prompt, "+pin "); short(text) {
			item := textsafe.Sane(text, 200)
			if s.pin() == nil && s.write(item.String(), orDash(s.prog), orDash(s.now)) == nil {
				s.block("Objective pinned: " + item.String())
				return true
			}
		}
	case strings.HasPrefix(prompt, "+ "):
		if text := strings.TrimPrefix(prompt, "+ "); short(text) {
			if n, err := s.queue().Add(s.id, textsafe.Sane(text, 200)); err == nil {
				s.ensureNotes()
				s.block("Queued (" + strconv.Itoa(n) + "): " + textsafe.Sane(text, 200).String())
				return true
			}
		}
	case strings.HasPrefix(prompt, "+inbox "):
		if text := strings.TrimLeft(strings.TrimPrefix(prompt, "+inbox "), " \t\n\r\f\v"); short(text) {
			if n, err := s.queue().Add(state.Inbox, textsafe.Sane(text, 200)); err == nil {
				s.block("Inbox (" + strconv.Itoa(n) + "): " + textsafe.Sane(text, 200).String())
				return true
			}
		}
	case prompt == "+take" || strings.HasPrefix(prompt, "+take "):
		n := strings.TrimPrefix(strings.TrimPrefix(prompt, "+take"), " ")
		if n == "" {
			n = "1"
		}
		if no, err := strconv.Atoi(n); err == nil && no >= 1 && n[0] != '+' && n[0] != '0' {
			s.ensureNotes()
			if item, err := s.queue().Move(state.Inbox, s.id, no, nil); err == nil {
				s.block("Took from the inbox: " + textsafe.Sane(item.String(), 200).String())
				return true
			}
		}
	}
	return false
}

// ensureNotes makes the notes file if there is none, so that the session resolves by id.
func (s *session) ensureNotes() {
	if !state.IsRegular(s.Store.Path(s.id, "txt")) {
		_ = s.write("-", "-", "-")
	}
}

// pin keeps the summarizer from rewriting the Objective.
func (s *session) pin() error {
	f, err := os.OpenFile(s.Store.Hidden(s.id, "pinned"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, state.FileMode)
	if err != nil {
		return err
	}
	return f.Close()
}

func (s *session) start() {
	// a resumed session is alive again; a compaction fires SessionStart in the middle of a turn, so it leaves the state alone
	if s.ev.Source != "compact" {
		s.setState("waiting")
	}
	if s.obj != "" || s.tp == "" {
		return
	}
	obj := textsafe.Sane(transcript.FirstPrompt(s.tp), 160).String()
	_ = s.write(orDash(obj), "-", "(resumed -- summarizing)")
	if obj != "" {
		s.summarizeNow()
	}
}

func (s *session) stop() {
	s.setState("waiting")
	if s.tp == "" {
		return
	}
	window := int64(defaultWindow)
	if v := s.Getenv("SITREP_DEBOUNCE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			n = 0 // a setting that is not a number turns the debounce off
		}
		window = int64(n)
	}
	now := s.Clock.Now().Unix()
	stamp := s.Store.Hidden(s.id, "stamp")
	fi, err := os.Stat(stamp)
	// a stamp from the future (a clock that moved back) is as good as none
	if err == nil && fi.ModTime().Unix() <= now && now-fi.ModTime().Unix() < window {
		// a Stop inside the window schedules one deferred run for its end, so a quick follow-up turn is never left stale
		wait := window - (now - fi.ModTime().Unix())
		if nonce, ok := s.claimPending(now, now+wait, window); ok {
			_ = s.Spawn("hook-summarize", "--wait", strconv.FormatInt(wait, 10), "--nonce", nonce, "--", string(s.id), s.tp)
		}
		return
	}
	touch(s.Store, stamp, now)
	s.summarizeNow()
}

// claimPending makes the one deferred run of a session. The pending file holds when the run is due and a nonce that names the child that
// owns it. A pending file whose time has long passed belongs to a child that died (a reboot, a kill), so it is replaced. The check and the
// replacement happen under a lock, so two Stops that find it stale cannot both win it.
func (s *session) claimPending(now, due, window int64) (string, bool) {
	unlock, err := state.Lock(s.Store.Hidden(s.id, "lock"))
	if err != nil {
		return "", false
	}
	defer unlock()
	path := s.Store.Hidden(s.id, "pending")
	if d, _, ok := ReadPending(path); ok {
		if now <= d+5 {
			return "", false
		}
	} else if fi, err := os.Stat(path); err == nil && now-fi.ModTime().Unix() <= window+5 {
		return "", false // an older version's file, with no time in it
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", false
	}
	nonce := hex.EncodeToString(raw[:])
	if err := s.Store.WriteFile(path, []byte(strconv.FormatInt(due, 10)+" "+nonce+"\n")); err != nil {
		return "", false
	}
	return nonce, true
}

// ReadPending reads a pending file: when the run is due, and the nonce of the child that owns it.
func ReadPending(path string) (due int64, nonce string, ok bool) {
	d, n, found := strings.Cut(state.FirstLine(path), " ")
	due, err := strconv.ParseInt(d, 10, 64)
	return due, n, found && err == nil && n != ""
}

// touch makes an empty file or raises its time.
func touch(st state.Store, path string, now int64) {
	if err := st.WriteFile(path, nil); err != nil {
		_ = err // a hook never fails
	}
}

// summarizeNow starts the summarizer, telling it the prompt count it is for.
func (s *session) summarizeNow() {
	since := state.FirstLine(s.Store.Hidden(s.id, "count"))
	_ = s.Spawn("hook-summarize", "--since", since, "--", string(s.id), s.tp)
}
