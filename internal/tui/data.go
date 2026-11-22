package tui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/lfeagan/claude-sitrep/internal/state"
)

// supportedVersion is the highest `sitrep ls --json` version this build understands.
const supportedVersion = 1

const (
	maxOutput   = 8 << 20 // bytes read from sitrep
	maxSessions = 500
	maxItems    = 100
)

// Snapshot is the document `sitrep ls --json -a` prints.
type Snapshot struct {
	Version   int       `json:"version"`
	Generated int64     `json:"generated"`
	Registry  string    `json:"registry"`
	Settings  Settings  `json:"settings"`
	Inbox     Queue     `json:"inbox"`
	Hidden    Hidden    `json:"hidden"`
	Sessions  []Session `json:"sessions"`
}

type Settings struct {
	QueueItems       int    `json:"queue_items"`
	HighlightSeconds int    `json:"highlight_seconds"`
	HighlightSGR     string `json:"highlight_sgr"`
	Theme            string `json:"theme"`
	Colors           string `json:"colors"`
}

type Queue struct {
	Count int      `json:"count"`
	Items []string `json:"items"`
}

type Hidden struct {
	Ended         int `json:"ended"`
	StrandedItems int `json:"stranded_items"`
}

// Label is the name a person is shown for a session: its alias, or the Short. Whatever is passed to a command is the ID, never this.
func (x Session) Label() string {
	if x.Alias != "" {
		return x.Alias
	}
	return x.Short
}

type Session struct {
	ID           string   `json:"id"`
	Short        string   `json:"short"`
	Alias        string   `json:"alias"`
	Dir          string   `json:"dir"`
	Cwd          string   `json:"cwd"`
	Running      bool     `json:"running"`
	State        string   `json:"state"`
	StateLabel   string   `json:"state_label"`
	Age          int64    `json:"age"`
	StateChanged int64    `json:"state_changed"`
	Idle         bool     `json:"idle"`
	NeedsInput   bool     `json:"needs_input"`
	Blocked      string   `json:"blocked"`
	Snoozed      bool     `json:"snoozed"`
	QuietSecs    *int64   `json:"quiet_secs"`
	PID          *int64   `json:"pid"`
	Pinned       bool     `json:"pinned"`
	Highlight    bool     `json:"highlight"`
	AttnFaded    bool     `json:"attn_faded"` // needs the user, but for so long that the row is no longer coloured
	Objective    string   `json:"objective"`
	Progress     string   `json:"progress"`
	Now          string   `json:"now"`
	Queue        Queue    `json:"queue"`
	Context      *Context `json:"context"`
}

// Context is how full the context window of a session was at its last status line.
type Context struct {
	UsedPercent float64 `json:"used_percent"`
	Window      int64   `json:"window"`
	Tokens      int64   `json:"tokens"`
	At          int64   `json:"at"`
	Age         int64   `json:"-"` // how long before the listing the status line last showed these numbers, worked out from the document's own time
}

// Left is the share of the window left, in whole per cent, rounded down.
func (c Context) Left() int { return int(math.Floor(100 - c.UsedPercent)) }

// decode reads one document. It tolerates fields it does not know, refuses a version it
// does not understand, and cleans every string, so nothing downstream sees raw text.
func decode(r io.Reader) (Snapshot, error) {
	var s Snapshot
	if err := json.NewDecoder(io.LimitReader(r, maxOutput)).Decode(&s); err != nil {
		return Snapshot{}, fmt.Errorf("reading sitrep output: %w", err)
	}
	if s.Version < 1 || s.Version > supportedVersion {
		return Snapshot{}, fmt.Errorf("sitrep speaks JSON version %d; this build understands %d", s.Version, supportedVersion)
	}
	s.sanitize()
	return s, nil
}

func (s *Snapshot) sanitize() {
	if s.Registry != "ok" {
		s.Registry = "unavailable"
	}
	s.Settings.HighlightSGR = clean(s.Settings.HighlightSGR)
	s.Settings.Theme, s.Settings.Colors = clean(s.Settings.Theme), clean(s.Settings.Colors)
	s.Hidden.Ended, s.Hidden.StrandedItems = max(s.Hidden.Ended, 0), max(s.Hidden.StrandedItems, 0)
	s.Inbox.sanitize()
	if len(s.Sessions) > maxSessions {
		s.Sessions = s.Sessions[:maxSessions]
	}
	for i := range s.Sessions {
		x := &s.Sessions[i]
		x.ID, x.Short, x.Dir, x.Cwd = clean(x.ID), clean(x.Short), clean(x.Dir), clean(x.Cwd)
		if !state.ValidAlias(x.Alias) { // the document is untrusted: only a name the grammar allows is drawn
			x.Alias = ""
		}
		x.StateLabel = clean(x.StateLabel)
		x.Objective, x.Progress, x.Now = clean(x.Objective), clean(x.Progress), clean(x.Now)
		switch x.State {
		case "working", "waiting", "ended", "running", "-":
		default:
			x.State = "-"
		}
		if x.Blocked != "permission" { // the document is untrusted: only the kinds sitrep writes are known
			x.Blocked = ""
		}
		x.Age = max(x.Age, 0)
		if c := x.Context; c != nil && !(c.UsedPercent >= 0 && c.UsedPercent <= 100 && c.Window >= 1) { // the document is untrusted: a NaN or a range error is no record
			x.Context = nil
		} else if c != nil {
			c.Age = 0
			if c.At > 0 && s.Generated > c.At {
				c.Age = s.Generated - c.At
			}
		}
		x.Queue.sanitize()
	}
}

func (q *Queue) sanitize() {
	if len(q.Items) > maxItems {
		q.Items = q.Items[:maxItems]
	}
	for i := range q.Items {
		q.Items[i] = clean(q.Items[i])
	}
	q.Count = max(q.Count, len(q.Items))
}

// limitedBuffer keeps at most max bytes and reports an error past that, so a runaway
// child cannot fill memory.
type limitedBuffer struct {
	buf  bytes.Buffer
	max  int
	over bool // set when a write was refused, whatever error the process then reports
}

var errTooBig = errors.New("output too large")

// keepFirst keeps the first max bytes and quietly drops the rest. It is for stderr, where
// a noisy child must never turn a good run into an error.
type keepFirst struct {
	buf bytes.Buffer
	max int
}

func (k *keepFirst) Write(p []byte) (int, error) {
	if room := k.max - k.buf.Len(); room > 0 {
		k.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.buf.Len()+len(p) > l.max {
		l.over = true
		return 0, errTooBig
	}
	return l.buf.Write(p)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
