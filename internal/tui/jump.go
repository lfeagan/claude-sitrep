package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Jumping to a session means finding the tmux pane that runs it. The session's process id
// comes from sitrep; tmux says which process started each pane, and the pane's shell is an
// ancestor of the session's process.

const (
	jumpTimeout = 3 * time.Second
	maxWalk     = 64 // parents followed from a process, as a guard against a loop
)

// pane is one tmux pane, by tmux's own ids, which are safe to pass back whatever the names are.
type pane struct {
	ID, SessionID, WindowID string
	PID                     int
	TTY                     string
}

var (
	paneRe   = regexp.MustCompile(`^%[0-9]+$`)
	windowRe = regexp.MustCompile(`^@[0-9]+$`)
	sessRe   = regexp.MustCompile(`^\$[0-9]+$`)
)

// listPanesFormat asks tmux for ids only, never for names or indexes.
const listPanesFormat = "#{pane_id} #{pane_pid} #{pane_tty} #{session_id} #{window_id}"

func parsePanes(out string) []pane {
	var panes []pane
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 5 {
			continue
		}
		pid, err := strconv.Atoi(f[1])
		if err != nil || pid <= 0 || !paneRe.MatchString(f[0]) || !sessRe.MatchString(f[3]) || !windowRe.MatchString(f[4]) {
			continue
		}
		panes = append(panes, pane{ID: f[0], PID: pid, TTY: f[2], SessionID: f[3], WindowID: f[4]})
	}
	return panes
}

// proc is one line of `ps -A -o pid=,ppid=,command=`.
type proc struct {
	PID, PPID int
	Args      []string
}

func parseProcs(out string) []proc {
	var procs []proc
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 == nil && err2 == nil && pid > 0 {
			procs = append(procs, proc{PID: pid, PPID: ppid, Args: f[2:]})
		}
	}
	return procs
}

// parentMap maps each process to its parent.
func parentMap(procs []proc) map[int]int {
	parents := make(map[int]int, len(procs))
	for _, p := range procs {
		parents[p.PID] = p.PPID
	}
	return parents
}

// attachClients finds the terminal processes `claude attach <prefix>` that show the
// session, for a session that runs in the background with no terminal of its own. The
// argument is a prefix of the session id; a client counts only if it names this session.
func attachClients(procs []proc, sessionID string) []int {
	var pids []int
	for _, p := range procs {
		if len(p.Args) >= 3 && filepath.Base(p.Args[0]) == "claude" && p.Args[1] == "attach" {
			if prefix := p.Args[2]; len(prefix) >= 6 && strings.HasPrefix(sessionID, prefix) {
				pids = append(pids, p.PID)
			}
		}
	}
	return pids
}

// ancestors lists pid and then each parent in turn, stopping at process 1, a loop, or maxWalk.
func ancestors(pid int, parents map[int]int) []int {
	chain := []int{}
	seen := map[int]bool{}
	for len(chain) < maxWalk && pid > 1 && !seen[pid] {
		chain = append(chain, pid)
		seen[pid] = true
		next, ok := parents[pid]
		if !ok {
			break
		}
		pid = next
	}
	return chain
}

// findPane returns the pane whose first process is the closest ancestor of the session.
func findPane(panes []pane, chain []int) (pane, bool) {
	for _, pid := range chain {
		for _, p := range panes {
			if p.PID == pid {
				return p, true
			}
		}
	}
	return pane{}, false
}

// normTTY makes the spellings of a terminal comparable: ttys003, pts/3 and /dev/ttys003.
func normTTY(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "?" || s == "??" || s == "-" {
		return ""
	}
	return "/dev/" + strings.TrimPrefix(s, "/dev/")
}

// paneByTTY is the fallback when the process walk fails: the one pane on the session's
// terminal. More than one, or none, is no answer.
func paneByTTY(panes []pane, tty string) (pane, bool) {
	tty = normTTY(tty)
	var found []pane
	for _, p := range panes {
		if tty != "" && normTTY(p.TTY) == tty {
			found = append(found, p)
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return pane{}, false
}

// jumpArgs is the one tmux call: select the window, then the pane, then switch the client, in
// that order so the client lands on the right pane. ";" is tmux's own separator.
func jumpArgs(p pane) []string {
	return []string{"select-window", "-t", p.WindowID, ";", "select-pane", "-t", p.ID, ";", "switch-client", "-t", p.SessionID}
}

// runner runs a command and returns its output; tests replace it.
type runner func(ctx context.Context, name string, args ...string) (string, error)

func realRunner(ctx context.Context, name string, args ...string) (string, error) {
	path, err := exec.LookPath(name) // never the current directory
	if err != nil {
		return "", fmt.Errorf("%s is not installed", name)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	out, errs := &limitedBuffer{max: 1 << 20}, &keepFirst{max: 512}
	cmd.Stdout, cmd.Stderr = out, errs
	cmd.WaitDelay = time.Second
	confine(cmd)
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(clean(firstLine(errs.buf.String()))); msg != "" {
			return "", fmt.Errorf("%s: %s", name, msg)
		}
		return "", fmt.Errorf("%s failed", name)
	}
	return out.buf.String(), nil
}

// jumpEnv says which multiplexers and terminal applications the dashboard itself runs inside.
type jumpEnv struct{ Tmux, Cmux, ITerm, Terminal bool }

func currentEnv() jumpEnv {
	return jumpEnv{
		Tmux: os.Getenv("TMUX") != "", Cmux: os.Getenv("CMUX_SURFACE_ID") != "" || os.Getenv("CMUX_WORKSPACE_ID") != "",
		ITerm: os.Getenv("TERM_PROGRAM") == "iTerm.app" || os.Getenv("ITERM_SESSION_ID") != "", Terminal: os.Getenv("TERM_PROGRAM") == "Apple_Terminal",
	}
}

// jump brings the terminal that shows s to the front, through tmux, cmux, iTerm2 or Terminal.app, whichever
// the dashboard runs inside, in that order: a pane inside tmux wins over the terminal application around it.
// It returns a short message for the status bar, and never guesses.
func jump(ctx context.Context, s Session, env jumpEnv, run runner) (string, error) {
	if !env.Tmux && !env.Cmux && !env.ITerm && !env.Terminal {
		return "", errors.New("not in tmux, cmux, iTerm2 or Terminal: run the dashboard inside one to jump")
	}
	if s.PID == nil || *s.PID <= 1 {
		return "", errors.New("sitrep does not know this session's process")
	}
	ctx, cancel := context.WithTimeout(ctx, jumpTimeout)
	defer cancel()
	snapshot, err := run(ctx, "ps", "-A", "-o", "pid=,ppid=,command=")
	if err != nil {
		return "", err
	}
	procs := parseProcs(snapshot)
	parents := parentMap(procs)
	// head is the process whose terminal is looked for: the session, or the client attached to it
	try := func(head int) (bool, error) {
		chains := [][]int{ancestors(head, parents)}
		if env.Tmux {
			if ok, err := jumpTmux(ctx, s, chains, run); err != nil || ok {
				return ok, err
			}
		}
		if env.Cmux {
			if ok, err := jumpCmux(ctx, chains, run); err != nil || ok {
				return ok, err
			}
		}
		if env.ITerm {
			if ok, err := jumpApp(ctx, iterm2, head, run); err != nil || ok {
				return ok, err
			}
		}
		if env.Terminal {
			return jumpApp(ctx, terminalApp, head, run)
		}
		return false, nil
	}
	if ok, err := try(int(*s.PID)); err != nil {
		return "", err
	} else if ok {
		return "jumped to " + s.Label(), nil
	}
	// a background session has no terminal above it; the client attached to it does. This comes
	// second, so that a look-alike `claude attach` process cannot get in the way of a real match.
	switch clients := attachClients(procs, s.ID); {
	case len(clients) == 1:
		if ok, err := try(clients[0]); err != nil {
			return "", err
		} else if ok {
			return "jumped to " + s.Label(), nil
		}
	case len(clients) > 1:
		return "", errors.New("several terminals are attached to this session")
	}
	hint := ""
	if len(s.ID) >= 8 {
		hint = "; if it runs in the background, open it with: claude attach " + s.ID[:8]
	}
	return "", errors.New("not in a pane here" + hint)
}

// jumpTmux selects the tmux pane that holds one of the chains. ok is false when none does.
func jumpTmux(ctx context.Context, s Session, chains [][]int, run runner) (ok bool, err error) {
	listing, err := run(ctx, "tmux", "list-panes", "-a", "-F", listPanesFormat)
	if err != nil {
		return false, err
	}
	panes := parsePanes(listing)
	var p pane
	for _, chain := range chains {
		if p, ok = findPane(panes, chain); ok {
			break
		}
	}
	if !ok {
		if tty, err := run(ctx, "ps", "-o", "tty=", "-p", strconv.FormatInt(*s.PID, 10)); err == nil {
			p, ok = paneByTTY(panes, tty)
		}
	}
	if !ok {
		return false, nil
	}
	_, err = run(ctx, "tmux", jumpArgs(p)...)
	return err == nil, err
}
