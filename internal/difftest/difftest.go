// Package difftest runs the last bash version of sitrep (the git tag bash-final) beside the Go one, so a port can be compared with the
// original on the same scenario. It goes away with the bash, once the outputs it checks are frozen as fixtures.
package difftest

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
	cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "LANG=" + os.Getenv("LANG"), "LC_ALL=" + os.Getenv("LC_ALL"), "TZ=UTC", "NO_COLOR=1", "TMPDIR=" + os.TempDir()}
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

// File is one state file as the comparison sees it.
type File struct {
	Mode    os.FileMode
	Content string
}

// State reads every regular file in the state folder, by name, with its mode and its content, in which the HOME is written as $HOME. A
// leftover temporary file (.sitrep.*) is kept under its own name, so a run that leaks one is a difference; the random part is dropped.
func State(t testing.TB, home string) map[string]File {
	t.Helper()
	dir := filepath.Join(home, ".claude", "sitrep")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]File{}
	for _, e := range entries {
		if !e.Type().IsRegular() || strings.HasSuffix(e.Name(), ".lock") {
			continue // a lock file is a Go-only artifact: the bash took no locks
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		fi, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		name := e.Name()
		if strings.HasPrefix(name, ".sitrep.") {
			name = ".sitrep.<temporary>"
		}
		m[name] = File{Mode: fi.Mode().Perm(), Content: strings.ReplaceAll(string(b), home, "$HOME")}
	}
	return m
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

// Diff runs the scenario with each runner in its own fresh HOME, and describes every difference in exit status, output or state files.
func Diff(t testing.TB, scenario Scenario, a, b Runner, norm ...func(Result) Result) []string {
	t.Helper()
	homeA, homeB := Home(t), Home(t)
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
