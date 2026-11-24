// Package snapshot is the one picture of everything sitrep knows: the sessions that ran in the last day, their notes and queues, and the
// inbox. `sitrep ls --json` is its encoding (version 1), and the dashboard reads it directly. Every string in it is a textsafe.Text, so
// no raw text reaches a consumer.
package snapshot

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/lfeagan/claude-sitrep/internal/clock"
	"github.com/lfeagan/claude-sitrep/internal/conf"
	"github.com/lfeagan/claude-sitrep/internal/live"
	"github.com/lfeagan/claude-sitrep/internal/queue"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// Version is the version of the document.
const Version = 1

// Window is how long ago a session may have changed and still be listed, in seconds.
const Window = 86400

const maxRead = 8 << 20

// Snapshot is the document `sitrep ls --json` prints. The field order is the document's order.
type Snapshot struct {
	Version   int       `json:"version"`
	Generated int64     `json:"generated"`
	Registry  string    `json:"registry"` // "ok", or "unavailable" when no live session could be read
	Settings  Settings  `json:"settings"`
	Hidden    Hidden    `json:"hidden"`
	Inbox     Queue     `json:"inbox"`
	Sessions  []Session `json:"sessions"`
}

// Settings are the values a consumer needs to draw the same way as the plain listing.
type Settings struct {
	QueueItems       int    `json:"queue_items"`
	HighlightSeconds int    `json:"highlight_seconds"`
	HighlightSGR     string `json:"highlight_sgr"`
	Theme            string `json:"theme"`  // SITREP_THEME, for the dashboard
	Colors           string `json:"colors"` // SITREP_COLORS, for the dashboard
}

// Hidden counts what a listing without ended sessions left out.
type Hidden struct {
	Ended         int `json:"ended"`
	StrandedItems int `json:"stranded_items"`
}

// Queue holds the true count and the items a cap allowed.
type Queue struct {
	Count int             `json:"count"`
	Items []textsafe.Text `json:"items"`
}

// Session is one session.
type Session struct {
	ID           state.ID      `json:"id"`
	Short        string        `json:"short"`
	Alias        string        `json:"alias"` // the name the user gave the session, or empty
	Running      bool          `json:"running"`
	State        string        `json:"state"`
	StateLabel   string        `json:"state_label"`
	Age          int64         `json:"age"`
	StateChanged int64         `json:"state_changed"`
	QuietSecs    *int64        `json:"quiet_secs"`
	Idle         bool          `json:"idle"`
	NeedsInput   bool          `json:"needs_input"`
	Blocked      string        `json:"blocked,omitempty"` // "permission" while the session waits at a dialog; absent otherwise
	Snoozed      bool          `json:"snoozed"`
	PID          *int          `json:"pid"`
	Pinned       bool          `json:"pinned"`
	Highlight    bool          `json:"highlight"`
	AttnFaded    bool          `json:"attn_faded,omitempty"`
	Dir          textsafe.Text `json:"dir"`
	Cwd          textsafe.Text `json:"cwd"`
	Objective    textsafe.Text `json:"objective"`
	Progress     textsafe.Text `json:"progress"`
	Now          textsafe.Text `json:"now"`
	Queue        Queue         `json:"queue"`
	Context      *Context      `json:"context"` // how full the context window was at the last status line, or null
}

// Context is how full a session's context window is, as its status line last said.
type Context struct {
	UsedPercent float64 `json:"used_percent"`
	Window      int64   `json:"window"`
	Tokens      int64   `json:"tokens"`
	At          int64   `json:"at"` // when the status line last showed these numbers, in epoch seconds
}

// Label is the name a person is shown for the session: its alias, or the Short.
func (s Session) Label() string {
	if s.Alias != "" {
		return s.Alias
	}
	return s.Short
}

// Builder makes snapshots.
type Builder struct {
	Store     state.Store
	ConfigDir string // Claude Code's folder, for transcripts and the registry
	Conf      conf.Conf
	Reg       live.Registry
	Clock     clock.Clock
}

// Options say what to include. Cap limits the items listed in each queue; below zero lists them all.
type Options struct {
	All bool
	Cap int
}

type found struct {
	id      string
	changed int64
}

// Build reads the state folder and the registry once and returns the snapshot.
func (b Builder) Build(ctx context.Context, opt Options) Snapshot {
	now := b.Clock.Now().Unix()
	set := b.Reg.Read(ctx)
	hlsec := b.Conf.HighlightSeconds()
	q := queue.Queue{Store: b.Store}
	snap := Snapshot{
		Version: Version, Generated: now, Registry: "unavailable",
		Settings: Settings{QueueItems: b.Conf.QueueItems(), HighlightSeconds: hlsec, HighlightSGR: b.Conf.HighlightSGR(), Theme: b.Conf.Theme(), Colors: b.Conf.Colors()},
		Sessions: []Session{},
	}
	if set.OK {
		snap.Registry = "ok"
	}
	snap.Inbox = readQueue(b.Store.Path(state.Inbox, "queue"), opt.Cap)

	for _, f := range b.sessions(now, set) {
		id := state.ID(f.id)
		word := live.Word(b.Store, f.id)
		dead := live.Dead(set, f.id, word, f.changed, now)
		if dead && !opt.All {
			snap.Hidden.Ended++
			snap.Hidden.StrandedItems += q.Count(id)
			continue
		}
		age := now - f.changed
		s := Session{ID: id, Short: shortOf(f.id), Alias: b.Store.Alias(id), Running: !dead, State: word, Age: age, StateChanged: f.changed}
		if c, ok := b.Store.Context(id); ok && !dead { // an ended session shows no window
			s.Context = &Context{UsedPercent: c.UsedPercent, Window: c.Window, Tokens: c.Tokens, At: c.At}
		}
		var quiet int64
		var hasQuiet bool
		if dead {
			s.State, s.StateLabel = "ended", "ended "+live.Age(age)
		} else {
			if word == "working" {
				quiet, hasQuiet = live.Quiet(b.ConfigDir, f.id, now)
			}
			s.StateLabel = live.Label(word, f.changed, now, quiet, hasQuiet)
			if strings.HasPrefix(s.StateLabel, "ended") { // alive in the registry, so a stale ended is not true
				s.StateLabel = "running" + strings.TrimPrefix(s.StateLabel, "ended")
			}
			if word == "ended" {
				s.State = "running"
			}
			if pid, ok := set.PID(f.id); ok {
				s.PID = &pid
			}
			if hasQuiet {
				s.QuietSecs = &quiet
			}
			// Claude has stopped and the turn is the user's until the next prompt, or it waits at a dialog until the user answers; a snooze
			// lasts until the time the session last changed, or the dialog opened
			since := f.changed
			if kind, at, ok := live.Blocked(b.Store, b.ConfigDir, f.id, word, now); ok {
				s.Blocked, since = kind, at
				s.StateLabel = "blocked " + live.Age(now-at)
			}
			if word == "waiting" || s.Blocked != "" {
				s.NeedsInput = true
				if snoozedAt(b.Store, id) == strconv.FormatInt(since, 10) {
					s.NeedsInput, s.Snoozed = false, true
				}
				fade := int64(b.Conf.AttnFade())
				s.AttnFaded = s.NeedsInput && fade > 0 && now-since > fade
			}
			s.Highlight = hlsec > 0 && age <= int64(hlsec)
		}
		s.Idle = strings.HasPrefix(s.StateLabel, "idle?")
		if !labelOK(s.StateLabel) {
			s.StateLabel = "-"
		}
		s.Pinned = state.IsRegular(b.Store.Path(id, "pinned"))

		notes := readText(b.Store.Path(id, "notes"))
		s.Objective, s.Progress, s.Now = field(notes, "Objective"), field(notes, "Progress"), field(notes, "Now")
		cwd := firstLine(readText(b.Store.Path(id, "cwd")))
		s.Dir, s.Cwd = dirOf(cwd), textsafe.Clean(cwd, 300)
		s.Queue = readQueue(b.Store.Path(id, "queue"), opt.Cap)
		snap.Sessions = append(snap.Sessions, s)
	}
	return snap
}

// sessions lists the sessions changed in the last day, and any session the registry shows alive however long it has been quiet (a
// weekend away leaves a waiting session waiting), newest first (by when the state last changed, else when the notes did).
func (b Builder) sessions(now int64, set live.Set) []found {
	var out []found
	for _, id := range b.Store.Sessions() {
		changed, ok := live.Changed(b.Store, string(id))
		if !ok || (now-changed >= Window && !set.Has(string(id))) {
			continue
		}
		out = append(out, found{string(id), changed})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].changed != out[j].changed {
			return out[i].changed > out[j].changed
		}
		return out[i].id > out[j].id
	})
	return out
}

func shortOf(id string) string {
	if len(id) > 4 {
		return id[:4]
	}
	return id
}

// labelOK says a state label is built from known words: letters or ?, then maybe a space and an age.
func labelOK(s string) bool {
	word, rest, hasRest := strings.Cut(s, " ")
	if word == "" {
		return false
	}
	for _, r := range word {
		if !(r >= 'a' && r <= 'z' || r == '?') {
			return false
		}
	}
	if !hasRest {
		return true
	}
	digits := strings.TrimRight(rest, "mhd")
	if len(rest)-len(digits) > 1 || digits == "" {
		return false
	}
	for _, r := range digits {
		if !(r >= '0' && r <= '9' || r == '<') {
			return false
		}
	}
	return true
}

// snoozedAt is the state-change time a `sitrep snooze` stored, as text.
func snoozedAt(s state.Store, id state.ID) string { return state.FirstLine(s.Path(id, "snooze")) }

// readText reads a file as text; a file that is missing, unreadable or not a regular file reads as empty, so one bad file does not blank
// the listing, and a pipe named like a session file does not hang it.
func readText(path string) string {
	b, _ := state.ReadFile(path, maxRead)
	return string(b)
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

// dirOf is the last part of a path, or ? when there is none.
func dirOf(cwd string) textsafe.Text {
	clean := textsafe.Clean(cwd, 0).String()
	parts := strings.FieldsFunc(clean, func(r rune) bool { return r == '/' })
	if len(parts) == 0 {
		return textsafe.Clean("?", 0)
	}
	return textsafe.Clean(parts[len(parts)-1], 0)
}

// field is the value of the first "Key:" line, cleaned and cut to 200 characters, the last of them an ellipsis when it was cut.
func field(notes, key string) textsafe.Text {
	for _, l := range strings.Split(notes, "\n") {
		if !strings.HasPrefix(l, key+":") {
			continue
		}
		v := l[strings.Index(l, ":")+1:]
		return textsafe.CleanMarked(strings.TrimLeft(v, " "), 200)
	}
	return textsafe.Clean("", 0)
}

func readQueue(path string, cap int) Queue {
	items := []textsafe.Text{}
	for _, l := range strings.Split(readText(path), "\n") {
		t := textsafe.Clean(l, queue.MaxItem)
		if strings.TrimSpace(t.String()) != "" {
			items = append(items, t)
		}
	}
	q := Queue{Count: len(items), Items: items}
	if cap >= 0 && len(items) > cap {
		q.Items = items[:cap]
	}
	return q
}

// Encode writes the document as indented JSON, the way `sitrep ls --json` prints it.
func Encode(w io.Writer, s Snapshot) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}
