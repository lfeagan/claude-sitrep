// Package difftest runs the last bash version of sitrep (the git tag bash-final) beside the Go one, so a port can be compared with the
// original on the same scenario. It goes away with the bash, once the outputs it checks are frozen as fixtures.
package difftest

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/lfeagan/claude-sitrep/internal/state"
)

// Tag is the git tag of the last commit that holds the bash scripts.
const Tag = "bash-final"

// Result is what one run of a command left behind.
type Result struct {
	Code   int
	Stdout string
	Stderr string
}

// Runner runs `sitrep args...` in a given HOME.
type Runner func(t testing.TB, home string, args ...string) Result

// unavailable skips the test on a developer machine, and fails it when SITREP_REQUIRE_ORACLE is set (CI sets it), where a skipped oracle
// would let a green run prove nothing.
func unavailable(t testing.TB, format string, args ...any) {
	t.Helper()
	if os.Getenv("SITREP_REQUIRE_ORACLE") != "" {
		t.Fatalf("difftest: "+format, args...)
	}
	t.Skipf("difftest: "+format, args...)
}

// Oracle materializes the bash scripts from the tag and returns a Runner for bin/sitrep. Without git, the tag, jq or perl the test is
// skipped, or failed in CI.
func Oracle(t testing.TB) Runner {
	t.Helper()
	for _, tool := range []string{"git", "bash", "jq", "perl", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			unavailable(t, "%s is not installed", tool)
		}
	}
	root, err := repoRoot()
	if err != nil {
		unavailable(t, "%v", err)
	}
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/tags/"+Tag).Run(); err != nil {
		unavailable(t, "no tag %s", Tag)
	}
	dir := t.TempDir()
	archive := exec.Command("git", "-C", root, "archive", Tag)
	untar := exec.Command("tar", "-x", "-C", dir)
	pipe, err := archive.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	untar.Stdin = pipe
	if err := untar.Start(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Run(); err != nil {
		t.Fatalf("git archive %s: %v", Tag, err)
	}
	if err := untar.Wait(); err != nil {
		t.Fatalf("tar: %v", err)
	}
	script := filepath.Join(dir, "bin", "sitrep")
	return func(t testing.TB, home string, args ...string) Result {
		t.Helper()
		env, stdin, rest := split(args)
		cmd := exec.Command("bash", append([]string{script}, rest...)...)
		if len(rest) == 1 && rest[0] == "hook" { // the hook and the status line were their own scripts
			cmd = exec.Command("bash", filepath.Join(dir, "hooks", "sitrep-hook.sh"))
		}
		if len(rest) == 1 && rest[0] == "statusline" {
			cmd = exec.Command("bash", filepath.Join(dir, "statusline", "statusline.sh"))
		}
		return run(t, home, env, stdin, cmd)
	}
}

// Binary builds the Go command into the test's temporary folder and returns a Runner for it.
func Binary(t testing.TB) Runner {
	t.Helper()
	r, err := BuildInto(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// BuildInto builds the Go command into dir, for a package that builds it once for all its tests (see TestMain).
func BuildInto(dir string) (Runner, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	bin := filepath.Join(dir, "sitrep")
	build := exec.Command("go", "build", "-o", bin, "./cmd/sitrep")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("go build: %v\n%s", err, out)
	}
	return func(t testing.TB, home string, args ...string) Result {
		t.Helper()
		env, stdin, rest := split(args)
		return run(t, home, env, stdin, exec.Command(bin, rest...))
	}, nil
}

// split takes the leading arguments that are not part of the command line off: env:NAME=value becomes an environment variable of the run, and
// stdin:text becomes its standard input.
func split(args []string) (env []string, stdin *string, rest []string) {
	for len(args) > 0 {
		switch {
		case strings.HasPrefix(args[0], "env:"):
			env = append(env, strings.TrimPrefix(args[0], "env:"))
		case strings.HasPrefix(args[0], "stdin:"):
			in := strings.TrimPrefix(args[0], "stdin:")
			stdin = &in
		default:
			return env, stdin, args
		}
		args = args[1:]
	}
	return env, stdin, args
}

func run(t testing.TB, home string, extra []string, stdin *string, cmd *exec.Cmd) Result {
	t.Helper()
	cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "LANG=" + os.Getenv("LANG"), "LC_ALL=" + os.Getenv("LC_ALL"), "TZ=UTC", "NO_COLOR=1", "SITREP_CHECK_UPDATES=0", "TMPDIR=" + os.TempDir()}
	cmd.Env = append(cmd.Env, extra...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if stdin != nil {
		cmd.Stdin = strings.NewReader(*stdin)
	}
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		code = ee.ExitCode()
	case err != nil:
		t.Fatalf("run %v: %v", cmd.Args, err)
	}
	return Result{Code: code, Stdout: out.String(), Stderr: errb.String()}
}

// Home makes an empty HOME with the folders Claude Code would have made.
func Home(t testing.TB) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude", "sitrep"), 0o700); err != nil {
		t.Fatal(err)
	}
	return home
}

// flatHomes holds the homes of runs of the bash oracle, which keeps every file of every session in the state folder. A home that is not in
// it is a home of the Go program, which keeps a folder for each session.
var flatHomes sync.Map

func isFlat(home string) bool { _, ok := flatHomes.Load(home); return ok }

func stateDir(home string) string { return filepath.Join(home, ".claude", "sitrep") }

// Path is where the file with this name of the old flat layout (<id>.txt, .<id>.state, inbox.queue) is in a home: where the bash keeps it,
// or where the Go program does.
func Path(home, name string) string {
	if !isFlat(home) {
		if l, ok := state.ParseLegacy(name); ok && !l.Obsolete {
			return state.Open(home).Path(l.ID, l.File)
		}
	}
	return filepath.Join(stateDir(home), name)
}

// Put writes a file named as in the old flat layout where the program of that home looks for it, making its folder, and returns the path.
// Scenarios seed their state with it, so that one scenario serves both programs.
func Put(t testing.TB, home, name, body string) string {
	t.Helper()
	p := Path(home, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// File is one state file as the comparison sees it.
type File struct {
	Mode    os.FileMode
	Content string
}

// flatName is the name a file of a session's folder has in the old flat layout, so that both programs' files can be compared by name.
func flatName(id, file string) string {
	switch file {
	case "notes":
		return id + ".txt"
	case "queue", "cwd":
		return id + "." + file
	}
	return "." + id + "." + file
}

// State reads every regular file of the state, by the name it has in the old flat layout, with its mode and its content, in which the HOME is
// written as $HOME. The Go layout (sessions/<id>/<file>) is read as the flat names, so both are compared as one. A leftover temporary file
// (.sitrep.*) is kept under its own name, so a run that leaks one is a difference; the random part is dropped. Lock files are left out: they
// are a Go-only artifact, as the bash took none. Anything unexpected (a folder in the root, a file in a session folder that is not one of ours)
// is kept under its path, so that it is a difference too.
func State(t testing.TB, home string) map[string]File {
	t.Helper()
	dir := stateDir(home)
	m := map[string]File{}
	add := func(path, name string) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(filepath.Base(name), ".sitrep.") {
			name = ".sitrep.<temporary>"
		}
		m[name] = File{Mode: fi.Mode().Perm(), Content: strings.ReplaceAll(string(b), home, "$HOME")}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		switch {
		case e.Name() == "sessions" && e.IsDir():
			ids, _ := os.ReadDir(filepath.Join(dir, "sessions"))
			for _, d := range ids {
				files, _ := os.ReadDir(filepath.Join(dir, "sessions", d.Name()))
				for _, f := range files {
					p := filepath.Join(dir, "sessions", d.Name(), f.Name())
					switch {
					case !d.IsDir() || !f.Type().IsRegular():
						m["?"+filepath.Join("sessions", d.Name(), f.Name())] = File{}
					case f.Name() == "lock":
					case strings.HasPrefix(f.Name(), ".sitrep."):
						add(p, ".sitrep.<temporary>")
					default:
						add(p, flatName(d.Name(), f.Name()))
					}
				}
			}
		case !e.Type().IsRegular():
			m["?"+e.Name()] = File{}
		case strings.HasSuffix(e.Name(), ".lock"):
		default:
			add(filepath.Join(dir, e.Name()), e.Name())
		}
	}
	return m
}

// CompareMigrated runs the scenario with the bash oracle, runs `sitrep migrate` (with the Go program) on the state it left, runs the scenario
// with the Go program, and fails when the two states differ: migration of what the bash wrote must give what the Go program writes.
func CompareMigrated(t *testing.T, scenario Scenario, goRunner Runner) {
	t.Helper()
	oracle := Oracle(t)
	homeA, homeB := Home(t), Home(t)
	flatHomes.Store(homeA, true)
	scenario(t, homeA, oracle)
	scenario(t, homeB, goRunner)
	if r := goRunner(t, homeA, "migrate"); r.Code != 0 {
		t.Fatalf("migrate: exit %d: %s%s", r.Code, r.Stdout, r.Stderr)
	}
	flatHomes.Delete(homeA) // it is the Go layout now
	sa, sb := State(t, homeA), State(t, homeB)
	for name, want := range sb {
		if got, ok := sa[name]; !ok || got != want {
			t.Errorf("state file %s differs\n  migrated bash: %+v\n  go:            %+v", name, got, want)
		}
	}
	for name := range sa {
		if _, ok := sb[name]; !ok {
			t.Errorf("state file %s exists only after migrating the bash state", name)
		}
	}
}

// Normalize makes a result comparable: the temporary HOME becomes $HOME.
func Normalize(r Result, home string) Result {
	return Result{Code: r.Code, Stdout: strings.ReplaceAll(r.Stdout, home, "$HOME"), Stderr: strings.ReplaceAll(r.Stderr, home, "$HOME")}
}

// Scenario drives a Runner in a fresh HOME and returns everything it saw: each result, then the state files.
type Scenario func(t testing.TB, home string, sitrep Runner) []Result

// Compare runs the scenario against the oracle and the Go binary and fails on any difference. A norm function makes results comparable
// (for example by blanking the time) before they are compared.
func Compare(t *testing.T, scenario Scenario, goRunner Runner, norm ...func(Result) Result) {
	t.Helper()
	for _, d := range Diff(t, scenario, Oracle(t), goRunner, norm...) {
		t.Error(d)
	}
}

// Diff runs the scenario with each runner in its own fresh HOME (a is the bash oracle, b the Go program), and describes every difference in exit status, output or state files.
func Diff(t testing.TB, scenario Scenario, a, b Runner, norm ...func(Result) Result) []string {
	t.Helper()
	homeA, homeB := Home(t), Home(t)
	flatHomes.Store(homeA, true) // a is the bash oracle, which keeps the flat layout
	ra, rb := scenario(t, homeA, a), scenario(t, homeB, b)
	for _, n := range norm {
		for i := range ra {
			ra[i] = n(ra[i])
		}
		for i := range rb {
			rb[i] = n(rb[i])
		}
	}
	if len(ra) != len(rb) {
		return []string{fmt.Sprintf("the scenario gave %d results and %d results", len(ra), len(rb))}
	}
	var diffs []string
	for i := range ra {
		x, y := Normalize(ra[i], homeA), Normalize(rb[i], homeB)
		if x != y {
			diffs = append(diffs, fmt.Sprintf("step %d differs: %s", i, describe(x, y)))
		}
	}
	sa, sb := State(t, homeA), State(t, homeB)
	if len(sa) != len(sb) {
		diffs = append(diffs, fmt.Sprintf("state files differ: first %v, second %v", keys(sa), keys(sb)))
	}
	for name, want := range sa {
		if got, ok := sb[name]; !ok || got != want {
			diffs = append(diffs, fmt.Sprintf("state file %s differs\n  first:  %+v\n  second: %+v", name, want, got))
		}
	}
	return diffs
}

var epoch = regexp.MustCompile(`^[0-9]{9,10}\n?$`)

var updateGolden = flag.Bool("update-golden", false, "rewrite the golden files of difftest.Golden")

// Golden runs the scenario with the Go program only and compares what it saw (each result, then the state files) with
// testdata/<name>.golden, or rewrites that file with -update-golden. It is for behaviour that has left the bash version on purpose, so the
// bash oracle can no longer be the judge: review the diff of a rewritten file as carefully as a change of code.
func Golden(t *testing.T, name string, scenario Scenario, goRunner Runner, norm ...func(Result) Result) {
	t.Helper()
	home := Home(t)
	rs := scenario(t, home, goRunner)
	var b strings.Builder
	for i, r := range rs {
		for _, n := range norm {
			r = n(r)
		}
		r = Normalize(r, home)
		fmt.Fprintf(&b, "== step %d: exit %d\n--- stdout\n%s--- stderr\n%s", i, r.Code, ensureNL(r.Stdout), ensureNL(r.Stderr))
	}
	st := State(t, home)
	names := make([]string, 0, len(st))
	for n := range st {
		names = append(names, n)
	}
	sort.Strings(names)
	b.WriteString("== state files\n")
	for _, n := range names {
		body := strings.ReplaceAll(st[n].Content, home, "$HOME")
		if epoch.MatchString(body) { // a time seeded from the clock, such as the one a snooze keeps
			body = "<epoch>\n"
		}
		fmt.Fprintf(&b, "--- %s (%v)\n%s", n, st[n].Mode, ensureNL(body))
	}
	got := b.String()
	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden file %s (run with -update-golden): %v", path, err)
	}
	if string(want) != got {
		t.Errorf("%s differs from the golden file; run with -update-golden and review the diff\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func ensureNL(s string) string {
	if s != "" && !strings.HasSuffix(s, "\n") {
		return s + "\n"
	}
	return s
}

func keys(m map[string]File) []string {
	var k []string
	for name := range m {
		k = append(k, name)
	}
	return k
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", errors.New("not inside the repository")
	}
	return strings.TrimSpace(string(out)), nil
}

// describe says what differs between two results: the exit status, or the first place two outputs part, with some context.
func describe(x, y Result) string {
	if x.Code != y.Code {
		return fmt.Sprintf("exit status %d against %d (stderr %q against %q)", x.Code, y.Code, x.Stderr, y.Stderr)
	}
	for _, p := range []struct{ name, a, b string }{{"stdout", x.Stdout, y.Stdout}, {"stderr", x.Stderr, y.Stderr}} {
		if p.a == p.b {
			continue
		}
		i := 0
		for i < len(p.a) && i < len(p.b) && p.a[i] == p.b[i] {
			i++
		}
		from := max(0, i-60)
		return fmt.Sprintf("%s parts at byte %d\n  first:  %q\n  second: %q", p.name, i, p.a[from:min(len(p.a), i+100)], p.b[from:min(len(p.b), i+100)])
	}
	return "equal"
}
