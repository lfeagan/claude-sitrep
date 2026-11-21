// Package live decides what is running: Claude Code's live-session registry, the state word a hook left, and how long a session's
// transcript has been quiet. The rules are the bash ones (live_ids, deadp, tquiet and stext in bin/sitrep).
package live

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/lfeagan/claude-sitrep/internal/runner"
	"github.com/lfeagan/claude-sitrep/internal/state"
)

// Grace is how long a session that is not in the registry may stay quiet before it counts as over: the registry lags a start and a /clear.
const Grace = 120

// IdleAfter is how long a working session's transcript may stay quiet before the session is shown as idle?.
const IdleAfter = 600

// Entry is one live session. PID is 0 when the process is in another pid space and cannot be judged here.
type Entry struct {
	ID  state.ID
	PID int
}

// Set is what the registry says is live. OK is false when nothing live could be read, which means the registry cannot be trusted either way.
type Set struct {
	Entries []Entry
	OK      bool
}

// Has says whether a live process has this session id.
func (s Set) Has(id string) bool {
	for _, e := range s.Entries {
		if string(e.ID) == id {
			return true
		}
	}
	return false
}

// PID is the first live process of a session, when it runs on this machine.
func (s Set) PID(id string) (int, bool) {
	for _, e := range s.Entries {
		if string(e.ID) == id {
			return e.PID, e.PID > 0
		}
	}
	return 0, false
}

// Registry reads ~/.claude/sessions/<pid>.json. Its files are untrusted input, so everything used is checked.
type Registry struct {
	Dir    string        // the sessions folder
	Run    runner.Runner // for ps
	Alive  func(pid int) bool
	Domain string // how Claude Code names this machine's pid space; empty when it cannot be worked out
}

// NewRegistry is the registry of a Claude config folder, with the real process checks.
func NewRegistry(configDir string, run runner.Runner) Registry {
	return Registry{Dir: filepath.Join(configDir, "sessions"), Run: run, Alive: processAlive, Domain: LocalDomain()}
}

func processAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// LocalDomain is how Claude Code names this machine's pid space.
func LocalDomain() string {
	if runtime.GOOS == "darwin" {
		return "darwin"
	}
	id, _ := os.ReadFile("/etc/machine-id")
	ns, _ := os.Readlink("/proc/self/ns/pid")
	if m := strings.TrimSpace(string(id)); m != "" && ns != "" {
		return "darwin:" + m + ":" + ns
	}
	return ""
}

type regFile struct {
	PID       any    `json:"pid"`
	SessionID any    `json:"sessionId"`
	ProcStart any    `json:"procStart"`
	PIDDomain string `json:"pidDomain"`
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

// Read lists the live sessions. OK is false when the registry holds no file, or no live session.
func (r Registry) Read(ctx context.Context) Set {
	entries, _ := os.ReadDir(r.Dir) // not a glob: the folder's own name may hold glob characters
	var set Set
	have := false
	for _, e := range entries {
		f := filepath.Join(r.Dir, e.Name())
		if !strings.HasSuffix(e.Name(), ".json") || !state.IsRegular(f) {
			continue
		}
		have = true
		b, err := state.ReadFile(f, 1<<20)
		if err != nil {
			continue
		}
		var rf regFile
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.UseNumber()
		if dec.Decode(&rf) != nil {
			continue
		}
		pid, err := strconv.Atoi(str(rf.PID))
		sid := str(rf.SessionID)
		if err != nil || pid < 1 || !state.SessionID(sid) {
			continue
		}
		if rf.PIDDomain != "" && r.Domain != "" && rf.PIDDomain != r.Domain {
			set.Entries = append(set.Entries, Entry{ID: state.ID(sid)}) // another pid space: cannot judge, count as alive
			continue
		}
		if !r.Alive(pid) {
			continue
		}
		if !r.sameProcess(ctx, pid, str(rf.ProcStart)) {
			continue // the pid was reused by another process
		}
		set.Entries = append(set.Entries, Entry{ID: state.ID(sid), PID: pid})
	}
	set.OK = have && len(set.Entries) > 0
	return set
}

func (r Registry) sameProcess(ctx context.Context, pid int, want string) bool {
	out, err := r.Run.Run(ctx, runner.Cmd{Name: "ps", Args: []string{"-o", "lstart=", "-p", strconv.Itoa(pid)}, Env: append(os.Environ(), "LC_ALL=C", "TZ=UTC")})
	if err != nil {
		return true
	}
	got, want := strings.Join(strings.Fields(string(out)), " "), strings.Join(strings.Fields(want), " ")
	return got == "" || want == "" || got == want
}

// Word is a session's state word from the file a hook wrote, from an allow-list (the file is untrusted input), else "-".
func Word(s state.Store, id string) string {
	if !state.SessionID(id) {
		return "-"
	}
	switch w := state.FirstLine(s.Path(state.ID(id), "state")); w {
	case "working", "waiting", "ended":
		return w
	}
	return "-"
}

// Changed is when a session's state last changed, in seconds since the epoch: the state file's time, else the notes file's.
func Changed(s state.Store, id string) (int64, bool) {
	if !state.SessionID(id) {
		return 0, false
	}
	for _, name := range []string{"state", "notes"} {
		if fi, err := os.Stat(s.Path(state.ID(id), name)); err == nil && fi.Mode().IsRegular() {
			return fi.ModTime().Unix(), true
		}
	}
	return 0, false
}

// Quiet is how many seconds the session's transcript has been unchanged (the shortest, if there are several). An interrupt fires no Stop
// hook, but the transcript still shows life.
func Quiet(configDir, id string, now int64) (int64, bool) {
	projects, _ := os.ReadDir(filepath.Join(configDir, "projects")) // not a glob: the folder's own name may hold glob characters
	var q int64
	found := false
	for _, p := range projects {
		m := filepath.Join(configDir, "projects", p.Name(), id+".jsonl")
		li, err := os.Lstat(m)
		if err != nil || !li.Mode().IsRegular() {
			continue // a link is not a transcript
		}
		n := now - li.ModTime().Unix()
		if !found || n < q {
			q, found = n, true
		}
	}
	return q, found
}

// BlockedSlack is how long after a block the transcript may still be written, in seconds: Claude Code writes it once, 0.3 s after a dialog opens.
const BlockedSlack = 3

// Block kinds a hook records.
const (
	BlockPermission = "permission"
)

// Blocked says whether a session is stopped at a dialog, and since when (seconds since the epoch). The permission hook wrote the file `blocked`;
// the block counts only while the session is working and its transcript has not been written since, so that an answered dialog and a crash leave nothing behind
// once the next transcript line is written, which for a long tool is when it finishes. A session without a readable transcript keeps the block until the file is removed.
func Blocked(s state.Store, configDir, id, word string, now int64) (kind string, at int64, ok bool) {
	if word != "working" || !state.SessionID(id) {
		return "", 0, false
	}
	f := strings.Fields(state.FirstLine(s.Path(state.ID(id), "blocked")))
	if len(f) != 2 || f[1] != BlockPermission {
		return "", 0, false
	}
	at, err := strconv.ParseInt(f[0], 10, 64)
	if err != nil || at <= 0 || at > now+BlockedSlack {
		return "", 0, false // a time from the future, after a clock moved back, would block the session for good
	}
	if quiet, has := Quiet(configDir, id, now); has && now-quiet > at+BlockedSlack {
		return "", 0, false
	}
	return f[1], at, true
}

// Age writes seconds as <1m, 5m, 3h or 2d.
func Age(secs int64) string {
	switch {
	case secs < 60:
		return "<1m"
	case secs < 3600:
		return fmt.Sprintf("%dm", secs/60)
	case secs < 86400:
		return fmt.Sprintf("%dh", secs/3600)
	}
	return fmt.Sprintf("%dd", secs/86400)
}

// Label is the state column: "waiting 3m". A working session whose transcript has been quiet for ten minutes is "idle? 12m".
func Label(word string, changed, now int64, quiet int64, hasQuiet bool) string {
	if word == "-" {
		return "-"
	}
	if word == "working" && hasQuiet && quiet > IdleAfter {
		return "idle? " + Age(quiet)
	}
	return word + " " + Age(now-changed)
}

// Dead says whether a session is over. With a readable registry a live process settles it; otherwise the session is over once it said so,
// or after two quiet minutes. Without a registry only its own word counts.
func Dead(set Set, id, word string, changed, now int64) bool {
	if set.OK {
		if set.Has(id) {
			return false
		}
		return word == "ended" || now-changed > Grace
	}
	return word == "ended"
}
