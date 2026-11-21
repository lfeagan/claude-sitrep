// Package tui is sitrep's full-screen dashboard: tabs for a Watch view and a Top view, a detail pane, jumping to a session's terminal, and the
// queue and prune screens. It reads a snapshot from an Engine and changes state only by running the same fixed sitrep commands, after a
// confirm, through that Engine; --read-only turns every one of them off.
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/lfeagan/claude-sitrep/internal/snapshot"
)

// engineName stands for the sitrep program in the commands the model runs. The Engine answers them, in this process.
const engineName = "sitrep"

// Engine is what the dashboard gets its data from and acts through.
type Engine struct {
	// Snapshot reads everything: all sessions, every queue item.
	Snapshot func(ctx context.Context) (snapshot.Snapshot, error)
	// Command runs one fixed sitrep command line (without the program name) and returns what it printed. An error carries its message.
	Command func(ctx context.Context, args ...string) (string, error)
}

// Options are the dashboard's settings.
type Options struct {
	Tab        string // watch or top
	Interval   time.Duration
	ReadOnly   bool
	NoBell     bool
	QuitOnJump bool
}

func tabByName(name string) (int, error) {
	switch name {
	case "", "watch":
		return tabWatch, nil
	case "top":
		return tabTop, nil
	}
	return 0, fmt.Errorf("unknown tab %q (watch or top)", name)
}

// FromSnapshot makes the dashboard's own copy of a snapshot, cleaned again as everything it shows is.
func FromSnapshot(s snapshot.Snapshot) Snapshot {
	out := Snapshot{Version: s.Version, Generated: s.Generated, Registry: s.Registry,
		Settings: Settings{QueueItems: s.Settings.QueueItems, HighlightSeconds: s.Settings.HighlightSeconds, HighlightSGR: s.Settings.HighlightSGR},
		Inbox:    queueFrom(s.Inbox), Hidden: Hidden{Ended: s.Hidden.Ended, StrandedItems: s.Hidden.StrandedItems}}
	for _, x := range s.Sessions {
		out.Sessions = append(out.Sessions, Session{
			ID: string(x.ID), Short: x.Short, Alias: x.Alias, Dir: x.Dir.String(), Cwd: x.Cwd.String(), Running: x.Running, State: x.State, StateLabel: x.StateLabel,
			Age: x.Age, StateChanged: x.StateChanged, Idle: x.Idle, NeedsInput: x.NeedsInput, Snoozed: x.Snoozed, QuietSecs: x.QuietSecs, PID: pidFrom(x.PID),
			Pinned: x.Pinned, Highlight: x.Highlight, Objective: x.Objective.String(), Progress: x.Progress.String(), Now: x.Now.String(), Queue: queueFrom(x.Queue)})
	}
	out.sanitize()
	return out
}

func pidFrom(p *int) *int64 {
	if p == nil {
		return nil
	}
	v := int64(*p)
	return &v
}

func queueFrom(q snapshot.Queue) Queue {
	out := Queue{Count: q.Count, Items: make([]string, 0, len(q.Items))}
	for _, it := range q.Items {
		out.Items = append(out.Items, it.String())
	}
	return out
}

func fetcher(eng Engine) func(context.Context) (Snapshot, error) {
	return func(ctx context.Context) (Snapshot, error) {
		s, err := eng.Snapshot(ctx)
		if err != nil {
			return Snapshot{}, err
		}
		return FromSnapshot(s), nil
	}
}

// routed sends the sitrep commands to the engine and everything else (tmux, ps, cmux) to the real programs.
func routed(eng Engine) runner {
	return func(ctx context.Context, name string, args ...string) (string, error) {
		if name == engineName {
			return eng.Command(ctx, args...)
		}
		return realRunner(ctx, name, args...)
	}
}

// Frame is one frame of a tab as plain text, for `sitrep status`.
func Frame(ctx context.Context, eng Engine, tab string, width int, showEnded bool) (string, error) {
	t, err := tabByName(tab)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	snap, err := fetcher(eng)(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(render(snap, t, "", viewOpts{Width: width, Height: 0, ShowEnded: showEnded, Theme: plainTheme()}), "\n"), nil
}

// Run shows the dashboard until the user quits.
func Run(opts Options, eng Engine) error {
	t, err := tabByName(opts.Tab)
	if err != nil {
		return err
	}
	if eng.Snapshot == nil || eng.Command == nil {
		return errors.New("the dashboard has no engine")
	}
	m := newModel(engineName, max(opts.Interval, time.Second), t)
	m.fetcher, m.run = fetcher(eng), routed(eng)
	m.quitJump, m.readOnly, m.bell = opts.QuitOnJump, opts.ReadOnly, !opts.NoBell
	_, err = tea.NewProgram(m).Run()
	return err
}
