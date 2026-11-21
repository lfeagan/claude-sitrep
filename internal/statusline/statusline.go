// Package statusline draws the sitrep block of Claude Code's status line: the optional upstream status line (such as Claude HUD) first,
// then the session's Objective, Progress and Now, and a queue line. Claude Code gives it the session as JSON on standard input.
package statusline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lfeagan/claude-sitrep/internal/conf"
	"github.com/lfeagan/claude-sitrep/internal/queue"
	"github.com/lfeagan/claude-sitrep/internal/runner"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
	"github.com/lfeagan/claude-sitrep/internal/update"
)

const maxInput = 16 << 20

// Line draws the status line.
type Line struct {
	Store state.Store
	Conf  conf.Conf
	Run   runner.Runner
	Env   []string // the environment of the upstream command, which gets SITREP_RENDERING=1 added
	// Version is this program's version; a newer release recorded by the daily check is announced after the queue line. Empty means no notice.
	Version string
}

// Render reads the event from in and writes the status line to out. It never fails: whatever goes wrong, what could be drawn is drawn.
func (l Line) Render(ctx context.Context, in io.Reader, out io.Writer) {
	defer func() { _ = recover() }()
	input, _ := io.ReadAll(io.LimitReader(in, maxInput))
	if cmd := l.Conf.Get("SITREP_HUD_CMD"); cmd != "" {
		up, _ := l.Run.Run(ctx, runner.Cmd{Name: "sh", Args: []string{"-c", cmd}, Stdin: strings.NewReader(string(input)), Env: append(append([]string(nil), l.Env...), "SITREP_RENDERING=1")})
		// the shell's command substitution drops trailing newlines; exactly one is added. The upstream line is the user's own command and
		// is passed as it is, colors included; only text that came from sessions is filtered.
		if s := strings.TrimRight(string(up), "\n"); s != "" {
			fmt.Fprintln(out, s)
		}
	}
	var ev struct {
		SessionID string `json:"session_id"`
	}
	if json.NewDecoder(strings.NewReader(string(input))).Decode(&ev) != nil || !state.SessionID(ev.SessionID) {
		return
	}
	id := state.ID(ev.SessionID)
	if b, err := state.ReadFile(l.Store.Path(id, "notes"), 1<<20); err == nil {
		fmt.Fprint(out, textsafe.Strip(string(b)))
	} else {
		fmt.Fprintln(out, "Objective: (set on your first prompt)")
	}
	qpath := l.Store.Path(id, "queue")
	q := queue.Queue{Store: l.Store}
	n, next := 0, ""
	if fi, err := os.Stat(qpath); err == nil && fi.Size() > 0 {
		n = q.Count(id)
		next = textsafe.Truncate(textsafe.Strip(state.FirstLine(qpath)), 90).String()
	}
	var summary string
	switch n {
	case 0:
		summary = "empty"
	case 1:
		summary = "1 item"
	default:
		summary = fmt.Sprintf("%d items", n)
	}
	if next != "" {
		summary += ", next: " + next
	}
	inbox := ""
	if c := q.Count(state.Inbox); c > 0 {
		inbox = fmt.Sprintf("  Inbox: %d", c)
	}
	fmt.Fprintf(out, "Queue: %s  [id %s]%s\n", summary, queue.Label(id), inbox)
	if l.Version != "" {
		if n := update.Notice(l.Store, l.Version); n != "" {
			fmt.Fprintln(out, n)
		}
	}
}
