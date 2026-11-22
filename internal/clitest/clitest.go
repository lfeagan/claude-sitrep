// Package clitest runs the sitrep program in a throwaway HOME, through scenarios of commands, and compares what it printed and left behind
// with golden files. The scenarios were first run against the last bash version of the program (the git tag bash-final), and their golden
// files hold what the two agreed on, except where the Go program was changed on purpose since.
package clitest

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
	"testing"

	"github.com/lfeagan/claude-sitrep/internal/state"
)

// Result is what one run of a command left behind.
type Result struct {
	Code   int
	Stdout string
	Stderr string
}

// Runner runs `sitrep args...` in a given HOME.
type Runner func(t testing.TB, home string, args ...string) Result

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

func stateDir(home string) string { return filepath.Join(home, ".claude", "sitrep") }

// Path is where the program keeps the file with this name of the old flat layout (<id>.txt, .<id>.state, inbox.queue) in a home. Scenarios
// are written with the flat names, and the program keeps a folder for each session.
func Path(home, name string) string {
	if l, ok := state.ParseLegacy(name); ok && !l.Obsolete {
		return state.Open(home).Path(l.ID, l.File)
	}
	return filepath.Join(stateDir(home), name)
}

// Put writes a file named as in the old flat layout where the program looks for it, making its folder, and returns the path.
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
// are not part of the state. Anything unexpected (a folder in the root, a file in a session folder that is not one of ours)
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

// Normalize makes a result comparable: the temporary HOME becomes $HOME.
func Normalize(r Result, home string) Result {
	return Result{Code: r.Code, Stdout: strings.ReplaceAll(r.Stdout, home, "$HOME"), Stderr: strings.ReplaceAll(r.Stderr, home, "$HOME")}
}

// Scenario drives a Runner in a fresh HOME and returns everything it saw: each result, then the state files.
type Scenario func(t testing.TB, home string, sitrep Runner) []Result

// The numbers in a state file that differ from run to run, because a scenario seeds them from the clock.
var (
	epoch   = regexp.MustCompile(`^[0-9]{9,10}\n?$`)
	pending = regexp.MustCompile(`^[0-9]{9,10} [0-9a-f]+\n?$`)
	context = regexp.MustCompile(`^(\S+ \S+ \S+) [0-9]{9,10}\n?$`)
)

// steady writes the volatile numbers of a state file as placeholders.
func steady(body string) string {
	switch {
	case epoch.MatchString(body):
		return "<epoch>\n"
	case pending.MatchString(body):
		return "<epoch> <nonce>\n"
	case context.MatchString(body):
		return context.ReplaceAllString(body, "$1 <epoch>\n")
	}
	return body
}

var updateGolden = flag.Bool("update-golden", false, "rewrite the golden files of clitest.Golden")

// Golden runs the scenario with the Go program only and compares what it saw (each result, then the state files) with
// testdata/<name>.golden, or rewrites that file with -update-golden. Review the diff of a rewritten file as carefully as a change of code.
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
		body := steady(strings.ReplaceAll(st[n].Content, home, "$HOME"))
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

// GoldenT is Golden with the file named after the test: its name with the characters that are not letters, digits or underscores replaced by
// underscores.
func GoldenT(t *testing.T, scenario Scenario, goRunner Runner, norm ...func(Result) Result) {
	t.Helper()
	Golden(t, strings.Trim(nameChars.ReplaceAllString(t.Name(), "_"), "_"), scenario, goRunner, norm...)
}

var nameChars = regexp.MustCompile(`[^A-Za-z0-9_]+`)

func ensureNL(s string) string {
	if s != "" && !strings.HasSuffix(s, "\n") {
		return s + "\n"
	}
	return s
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", errors.New("not inside the repository")
	}
	return strings.TrimSpace(string(out)), nil
}
