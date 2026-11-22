package cli

import (
	"context"
	"errors"

	"github.com/lfeagan/claude-sitrep/internal/snapshot"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
	"github.com/lfeagan/claude-sitrep/internal/tui"
)

// demoEngine feeds the dashboard made-up sessions, so it can be tried before sitrep is installed. It reads nothing from disk, and every
// command it is asked to run fails.
func (e Env) demoEngine() tui.Engine {
	return tui.Engine{
		Snapshot: func(context.Context) (snapshot.Snapshot, error) { return demoSnapshot(e.Clock.Now().Unix()), nil },
		Command: func(context.Context, ...string) (string, error) {
			return "", errors.New("this is a demo: nothing is changed")
		},
	}
}

type demoSession struct {
	id, dir, state, label    string
	age                      int64 // seconds since the state changed
	needs, blocked, snoozed  bool
	ended                    bool
	objective, progress, now string
	queue                    []string
	left                     float64 // the share of the context window left
}

var demoSessions = []demoSession{
	{"a1f3c9d2", "web", "waiting", "waiting", 190, true, false, false, false, "Dark mode for the settings page", "Theme tokens are in; the toggle works in Chrome", "Waiting for you: should the choice be saved per device or per account?", []string{"check the focus ring in dark mode"}, 71},
	{"b7e20c41", "api", "waiting", "waiting", 40, true, true, false, false, "Add pagination to list_orders", "Cursor pagination written, tests pass", "Waiting at a permission dialog: run `go test ./...`", nil, 55},
	{"c9d4a6e8", "infra", "working", "working", 20, false, false, false, false, "Move the nightly job to the new runner", "Image built; the schedule is copied", "Editing .github/workflows/nightly.yml", []string{"remove the old cron entry", "tell the team in the channel"}, 38},
	{"d0b1f5a7", "docs", "working", "working", 75, false, false, false, false, "Split the README into pages", "Install and Queue are done", "Writing docs/SETTINGS.md", nil, 64},
	{"e5a8b3c0", "mobile-app", "working", "working", 310, false, false, false, false, "Fix the crash when the camera permission is denied", "Reproduced on Android 14", "Reading CameraScreen.kt", []string{"add a test for the denied case", "update the changelog"}, 12},
	{"f2c7d91b", "data", "working", "idle? 25m", 1500, false, false, false, false, "Backfill the events table", "First 3 of 8 partitions copied", "Running the partition copy", nil, 29},
	{"0a9e4d6f", "web", "waiting", "waiting", 3600, true, false, true, false, "Replace the date picker", "Picker swapped; styling is left", "Waiting for you: keep the old keyboard shortcuts?", nil, 80},
	{"1c6b8e3a", "tools", "ended", "ended 8m", 480, false, false, false, true, "Rename the lint target", "Done and merged", "-", nil, 90},
}

// demoSnapshot is the document the dashboard reads, with the times counted back from now.
func demoSnapshot(now int64) snapshot.Snapshot {
	s := snapshot.Snapshot{Version: snapshot.Version, Generated: now, Registry: "ok",
		Settings: snapshot.Settings{QueueItems: 5, HighlightSeconds: 60},
		Inbox:    demoQueue([]string{"look at the flaky test in CI", "ask about the release date"})}
	for _, d := range demoSessions {
		x := snapshot.Session{
			ID: demoID(d.id), Short: d.id[:4], Running: !d.ended, State: d.state, StateLabel: d.label,
			Age: d.age, StateChanged: now - d.age, Idle: len(d.label) > 5 && d.label[:5] == "idle?",
			NeedsInput: d.needs && !d.snoozed, Snoozed: d.snoozed, Highlight: d.age <= 60 && !d.ended,
			Dir: textsafe.Sane(d.dir, 60), Cwd: textsafe.Sane("/home/you/"+d.dir, 300),
			Objective: textsafe.Sane(d.objective, 100), Progress: textsafe.Sane(d.progress, 140), Now: textsafe.Sane(d.now, 140),
			Queue:   demoQueue(d.queue),
			Context: &snapshot.Context{UsedPercent: 100 - d.left, Window: 200000, Tokens: int64((100 - d.left) * 2000), At: now},
		}
		if d.blocked {
			x.Blocked = "permission"
		}
		if d.ended {
			x.Context = nil
		}
		s.Sessions = append(s.Sessions, x)
	}
	return s
}

func demoID(s string) state.ID {
	id, _ := state.ParseID(s)
	return id
}

func demoQueue(items []string) snapshot.Queue {
	q := snapshot.Queue{Count: len(items), Items: make([]textsafe.Text, 0, len(items))}
	for _, it := range items {
		q.Items = append(q.Items, textsafe.Sane(it, 200))
	}
	return q
}
