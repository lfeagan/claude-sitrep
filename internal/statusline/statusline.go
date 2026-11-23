// Package statusline draws the sitrep block of Claude Code's status line: the optional upstream status line (such as Claude HUD) first,
// then the session's Objective, Progress and Now, and a queue line. Claude Code gives it the session as JSON on standard input.
package statusline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

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
	// Now is the time recorded with the context window; time.Now when nil.
	Now func() time.Time
	// UpstreamLimit is how long the upstream command may take; UpstreamTimeout when zero.
	UpstreamLimit time.Duration
}

// UpstreamTimeout bounds the upstream status line: a prompt must not wait for a command that hangs.
const UpstreamTimeout = 3 * time.Second

// Render reads the event from in and writes the status line to out. It never fails: whatever goes wrong, what could be drawn is drawn.
func (l Line) Render(ctx context.Context, in io.Reader, out io.Writer) {
	defer func() { _ = recover() }()
	input, _ := io.ReadAll(io.LimitReader(in, maxInput))
	if cmd := l.Conf.Get("SITREP_HUD_CMD"); cmd != "" {
		limit := l.UpstreamLimit
		if limit <= 0 {
			limit = UpstreamTimeout
		}
		uctx, cancel := context.WithTimeout(ctx, limit)
		// its own process group, so that a timeout kills the shell and whatever the shell started
		up, _ := l.Run.Run(uctx, runner.Cmd{Name: "sh", Args: []string{"-c", cmd}, Stdin: strings.NewReader(string(input)), Env: append(append([]string(nil), l.Env...), "SITREP_RENDERING=1"), NewGroup: true})
		timedOut := uctx.Err() != nil
		cancel()
		// the shell's command substitution drops trailing newlines; exactly one is added. The upstream line is the user's own command and
		// is passed as it is, colors included; only text that came from sessions is filtered. What a command printed before it was cut off
		// at the limit is not drawn: the sitrep lines below are.
		if s := strings.TrimRight(string(up), "\n"); s != "" && !timedOut {
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
	show := l.Conf.StatusLines()
	if b, err := state.ReadFile(l.Store.Path(id, "notes"), 1<<20); err == nil {
		// each line of the notes is cut, as the queue's next item is: a notes file is not trusted to be short
		fmt.Fprint(out, onlyShown(cutLines(textsafe.Strip(string(b)).String(), 300), show))
	} else if show["objective"] {
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
	// the id comes first, so a terminal that cuts a long line cuts the next item and not the id
	name := queue.Label(id)
	if a := l.Store.Alias(id); a != "" {
		name += " " + a // the id stays: it is the stable name, and a name that moves must not mislead
	}
	if show["queue"] {
		fmt.Fprintf(out, "Queue [id %s]: %s%s\n", name, summary, inbox)
	}
	if l.Version != "" {
		if n := update.Notice(l.Store, l.Version); n != "" {
			fmt.Fprintln(out, n)
		}
	}
	l.recordContext(id, input) // last, and best effort: the line is already written
}

// cutLines cuts every line of text to at most n characters.
func cutLines(text string, n int) string {
	lines := strings.SplitAfter(text, "\n")
	for i, l := range lines {
		body := strings.TrimSuffix(l, "\n")
		if r := []rune(body); len(r) > n {
			lines[i] = string(r[:n]) + l[len(body):]
		}
	}
	return strings.Join(lines, "")
}

// onlyShown drops the lines of the notes that the user has turned off; any other line stays.
func onlyShown(notes string, show map[string]bool) string {
	var out strings.Builder
	for _, ln := range strings.SplitAfter(notes, "\n") {
		k, _, _ := strings.Cut(ln, ":")
		switch k = strings.ToLower(k); k {
		case "objective", "progress", "now":
			if !show[k] {
				continue
			}
		}
		out.WriteString(ln)
	}
	return out.String()
}

// window is the part of Claude Code's input that says how full the context window is.
type window struct {
	ContextWindow *struct {
		Size           *float64 `json:"context_window_size"`
		UsedPercentage *float64 `json:"used_percentage"`
		CurrentUsage   *struct {
			Input         float64 `json:"input_tokens"`
			CacheCreation float64 `json:"cache_creation_input_tokens"`
			CacheRead     float64 `json:"cache_read_input_tokens"`
		} `json:"current_usage"`
	} `json:"context_window"`
}

// ContextOf reads the context window of a session from the status line's input, and says whether there is one. The percentage is
// used_percentage when that is above 0: Claude Code sends 0 or null before the first answer though current_usage is real, so otherwise it
// is the tokens over the size. A window and no usage (a new session, or just after /compact) is empty, 0 used.
func ContextOf(input []byte, now time.Time) (state.Context, bool) {
	var w window
	if json.Unmarshal(input, &w) != nil || w.ContextWindow == nil || w.ContextWindow.Size == nil {
		return state.Context{}, false
	}
	cw := w.ContextWindow
	size := *cw.Size
	if size != math.Trunc(size) || size < 1 || size > state.MaxWindow {
		return state.Context{}, false
	}
	c := state.Context{Window: int64(size), At: now.Unix()}
	if u := cw.CurrentUsage; u != nil {
		sum := u.Input + u.CacheCreation + u.CacheRead
		if sum < 0 || sum > state.MaxTokens || sum != math.Trunc(sum) {
			return state.Context{}, false
		}
		c.Tokens = int64(sum)
	}
	switch {
	case cw.UsedPercentage != nil && *cw.UsedPercentage > 0:
		c.UsedPercent = *cw.UsedPercentage
	case c.Tokens > 0:
		c.UsedPercent = math.Min(float64(c.Tokens)/size*100, 100)
	}
	return c, c.Valid()
}

func (l Line) recordContext(id state.ID, input []byte) {
	now := time.Now
	if l.Now != nil {
		now = l.Now
	}
	if c, ok := ContextOf(input, now()); ok {
		_ = l.Store.RecordContext(id, c) // a failed write is not the line's concern
	}
}
