package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
}

type Queue struct {
	Count int      `json:"count"`
	Items []string `json:"items"`
}

type Hidden struct {
	Ended         int `json:"ended"`
	StrandedItems int `json:"stranded_items"`
}

type Session struct {
	ID           string `json:"id"`
	Short        string `json:"short"`
	Dir          string `json:"dir"`
	Cwd          string `json:"cwd"`
	Running      bool   `json:"running"`
	State        string `json:"state"`
	StateLabel   string `json:"state_label"`
	Age          int64  `json:"age"`
	StateChanged int64  `json:"state_changed"`
	Idle         bool   `json:"idle"`
	NeedsInput   bool   `json:"needs_input"`
	QuietSecs    *int64 `json:"quiet_secs"`
	Pinned       bool   `json:"pinned"`
	Highlight    bool   `json:"highlight"`
	Objective    string `json:"objective"`
	Progress     string `json:"progress"`
	Now          string `json:"now"`
	Queue        Queue  `json:"queue"`
}

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
	s.Hidden.Ended, s.Hidden.StrandedItems = max(s.Hidden.Ended, 0), max(s.Hidden.StrandedItems, 0)
	s.Inbox.sanitize()
	if len(s.Sessions) > maxSessions {
		s.Sessions = s.Sessions[:maxSessions]
	}
	for i := range s.Sessions {
		x := &s.Sessions[i]
		x.ID, x.Short, x.Dir, x.Cwd = clean(x.ID), clean(x.Short), clean(x.Dir), clean(x.Cwd)
		x.StateLabel = clean(x.StateLabel)
		x.Objective, x.Progress, x.Now = clean(x.Objective), clean(x.Progress), clean(x.Now)
		switch x.State {
		case "working", "waiting", "ended", "running", "-":
		default:
			x.State = "-"
		}
		x.Age = max(x.Age, 0)
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

// fetch runs `sitrep ls --json -a` and decodes it. The arguments are a slice, never a
// shell line, and the call is cut off after timeout.
func fetch(ctx context.Context, bin string, timeout time.Duration) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "ls", "--json", "-a")
	out := &limitedBuffer{max: maxOutput}
	errs := &keepFirst{max: 4096}
	cmd.Stdout, cmd.Stderr = out, errs
	cmd.WaitDelay = time.Second
	confine(cmd)
	if err := cmd.Run(); err != nil {
		switch {
		case out.over: // the child usually dies of SIGPIPE once the pipe is closed, so look here first
			return Snapshot{}, errors.New("sitrep printed too much")
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return Snapshot{}, fmt.Errorf("sitrep took longer than %s", timeout)
		}
		msg := strings.TrimSpace(clean(firstLine(errs.buf.String())))
		if msg == "" {
			msg = err.Error()
		}
		return Snapshot{}, fmt.Errorf("sitrep failed: %s", msg)
	}
	return decode(&out.buf)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// findSitrep locates the sitrep command: $SITREP_BIN, then the one next to this program,
// then PATH. It never looks in the current directory.
func findSitrep() (string, error) {
	if p := os.Getenv("SITREP_BIN"); p != "" {
		if !isExecutable(p) {
			return "", fmt.Errorf("SITREP_BIN %q is not an executable file", p)
		}
		return p, nil
	}
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		if p := filepath.Join(filepath.Dir(exe), "sitrep"); isExecutable(p) {
			return p, nil
		}
	}
	p, err := exec.LookPath("sitrep")
	if err != nil {
		return "", errors.New("cannot find the sitrep command: put it on PATH, or set SITREP_BIN")
	}
	return p, nil
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0o111 != 0
}

func nilContext() context.Context { return context.Background() }
