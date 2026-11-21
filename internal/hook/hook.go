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
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lfeagan/claude-sitrep/internal/clock"
	"github.com/lfeagan/claude-sitrep/internal/histlog"
	"github.com/lfeagan/claude-sitrep/internal/live"
	"github.com/lfeagan/claude-sitrep/internal/notes"
	"github.com/lfeagan/claude-sitrep/internal/queue"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
	"github.com/lfeagan/claude-sitrep/internal/transcript"
	"github.com/lfeagan/claude-sitrep/internal/update"
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
	// CheckUpdates lets an event start the daily look for a newer release; the zero value does not.
	CheckUpdates bool
	// Spawn starts `sitrep <args>` detached from the hook, which must not wait for it.
	Spawn func(args ...string) error
	// Log records the changes the hook makes; the zero Log is off.
	Log histlog.Log
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
func (s *session) queue() queue.Queue { return queue.Queue{Store: s.Store, Log: s.Log} }

func (s *session) setState(word string) {
	_ = s.Store.WriteFile(s.Store.Path(s.id, "state"), []byte(word+"\n")) // a hook never fails
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

// markBlocked records that the session waits at a permission dialog. It prints nothing, as a hook for this event could otherwise answer the dialog,
// and stores only the time and the kind: the tool and its input are text from a model.
func (s *session) markBlocked() {
	_ = s.Store.WriteFile(s.Store.Path(s.id, "blocked"), []byte(strconv.FormatInt(s.Clock.Now().Unix(), 10)+" "+live.BlockPermission+"\n"))
}

// unblock removes the record of a dialog.
func (s *session) unblock() {
	_ = os.Remove(s.Store.Path(s.id, "blocked")) // a hook never fails
}

func (s *session) run() {
	if s.ev.HookEventName == "PermissionRequest" {
		s.markBlocked()
		return
	}
	if s.ev.HookEventName == "SessionEnd" {
		s.unblock()
		s.setState("ended")
		s.Log.Record(histlog.Entry{Op: histlog.SessionEnd, ID: s.id})
		return
	}
	if s.CheckUpdates && update.Claim(s.Store, s.Clock.Now()) {
		_ = s.Spawn("update-check")
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
		s.unblock()
		s.prompt()
	case "SessionStart":
		s.unblock()
		s.start()
	case "Stop":
		s.unblock()
		s.stop()
	}
}

func (s *session) prompt() {
	prompt := strings.TrimRight(s.ev.Prompt, "\n") // the shell's command substitution
	c, _ := strconv.Atoi(state.FirstLine(s.Store.Path(s.id, "count")))
	_ = s.Store.WriteFile(s.Store.Path(s.id, "count"), []byte(strconv.Itoa(c+1)+"\n")) // for the stale-summary check
	if s.plus(prompt) {
		return
	}
	s.setState("working") // only a prompt that goes to Claude starts work
	obj := s.obj
	if obj == "" {
		obj = textsafe.Sane(transcript.FirstPrompt(s.tp), 160).String()
	}
	if wrapped(prompt) { // a notification Claude Code sent, not something the user said: it says nothing about what is happening
		now := orDash(s.now)
		if label := notice(prompt); label != "" {
			now = label
		}
		_ = s.write(orDash(obj), orDash(s.prog), now)
		return
	}
	said := untag(prompt)
	if obj == "" {
		obj = orDash(textsafe.Sane(said, 160).String())
	}
	now := textsafe.Sane(said, 140).String()
	if strings.TrimSpace(now) == "" {
		now = orDash(s.now) // nothing but markup or placeholders
	}
	_ = s.write(obj, orDash(s.prog), now)
}

var tagRe = regexp.MustCompile(`(?:<|&lt;)/?[A-Za-z][A-Za-z0-9_-]*(?:\s[^<>]*?)?(?:>|&gt;)|\[(?:Image|Pasted text) #[0-9]+[^\]\n]*\]`)

// untag drops what is not the user's words: XML-like tags, which a pasted notification carries in the middle of the user's own words, and
// the placeholders Claude Code writes for a pasted image or text ("[Image #5]"). The text between them stays.
func untag(s string) string {
	if !tagRe.MatchString(s) {
		return s
	}
	return strings.Join(strings.Fields(tagRe.ReplaceAllString(s, " ")), " ")
}

// notices name the tags whose messages say what happened, as the line Now shows. Any other tag leaves Now as it was.
var notices = map[string]string{"task-notification": "Background task finished", "agent-message": "Subagent reported back"}

// notice is the Now line for a wrapped prompt, or empty when the tag is not one that is known.
func notice(prompt string) string {
	head := strings.TrimLeftFunc(prompt, unicode.IsSpace)
	head = strings.TrimPrefix(strings.TrimPrefix(head, "&lt;"), "<")
	for tag, label := range notices {
		if rest, ok := strings.CutPrefix(head, tag); ok && (rest == "" || !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-", rune(rest[0]))) {
			return label
		}
	}
	return ""
}

// wrapped says a prompt starts with a tag, as the task notifications, agent hand-backs and command wrappers that Claude Code sends do. The
// transcript scan skips the same messages.
func wrapped(prompt string) bool {
	head := strings.TrimLeftFunc(prompt, unicode.IsSpace)
	return strings.HasPrefix(head, "<") || strings.HasPrefix(head, "&lt;")
}

// plus handles +pin, +, +inbox and +take, and says whether the prompt was answered here. Anything that is not a short single line, or whose
// write fails, goes on to Claude untouched.
func (s *session) plus(prompt string) bool {
	switch {
	case strings.HasPrefix(prompt, "+pin "):
		if text := strings.TrimPrefix(prompt, "+pin "); short(text) {
			item := textsafe.Sane(text, 200)
			if s.pin() == nil && s.write(item.String(), orDash(s.prog), orDash(s.now)) == nil {
				s.Log.Record(histlog.Entry{Op: histlog.Pin, ID: s.id, Text: item})
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
	if !state.IsRegular(s.Store.Path(s.id, "notes")) {
		_ = s.write("-", "-", "-")
	}
}

// pin keeps the summarizer from rewriting the Objective.
func (s *session) pin() error {
	if err := s.Store.EnsureSession(s.id); err != nil {
		return err
	}
	f, err := os.OpenFile(s.Store.Path(s.id, "pinned"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, state.FileMode)
	if err != nil {
		return err
	}
	return f.Close()
}

func (s *session) start() {
	// a resumed session is alive again; a compaction fires SessionStart in the middle of a turn, so it leaves the state alone
	if s.ev.Source != "compact" {
		s.setState("waiting")
		s.Log.Record(histlog.Entry{Op: histlog.SessionStart, ID: s.id})
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
	stamp := s.Store.Path(s.id, "stamp")
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
	unlock, err := state.Lock(s.Store.Path(s.id, "lock"))
	if err != nil {
		return "", false
	}
	defer unlock()
	path := s.Store.Path(s.id, "pending")
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
	since := state.FirstLine(s.Store.Path(s.id, "count"))
	_ = s.Spawn("hook-summarize", "--since", since, "--", string(s.id), s.tp)
}
