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

// Oracle materializes the bash scripts from the tag and returns a Runner for bin/sitrep. It skips the test when git, the tag, jq or perl is missing.
func Oracle(t testing.TB) Runner {
	t.Helper()
	for _, tool := range []string{"git", "bash", "jq", "perl", "tar"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("difftest: %s is not installed", tool)
		}
	}
	root, err := repoRoot()
	if err != nil {
		t.Skipf("difftest: %v", err)
	}
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/tags/"+Tag).Run(); err != nil {
		t.Skipf("difftest: no tag %s", Tag)
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
		return run(t, home, exec.Command("bash", append([]string{script}, args...)...))
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
		return run(t, home, exec.Command(bin, args...))
	}, nil
}

func run(t testing.TB, home string, cmd *exec.Cmd) Result {
	t.Helper()
	cmd.Env = append(os.Environ(), "HOME="+home, "CLAUDE_CONFIG_DIR=", "SITREP_QUEUE_ITEMS=", "NO_COLOR=1")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
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

// State reads every regular file in the state folder, by name.
func State(t testing.TB, home string) map[string]string {
	t.Helper()
	dir := filepath.Join(home, ".claude", "sitrep")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, e := range entries {
		if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), ".sitrep.") {
			continue // temporaries have random names
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		m[e.Name()] = string(b)
	}
	return m
}

// Normalize makes a result comparable: the temporary HOME becomes $HOME.
func Normalize(r Result, home string) Result {
	return Result{Code: r.Code, Stdout: strings.ReplaceAll(r.Stdout, home, "$HOME"), Stderr: strings.ReplaceAll(r.Stderr, home, "$HOME")}
}

// Scenario drives a Runner in a fresh HOME and returns everything it saw: each result, then the state files.
type Scenario func(t testing.TB, home string, sitrep Runner) []Result

// Compare runs the scenario against the oracle and the Go binary and fails on any difference.
func Compare(t *testing.T, scenario Scenario, goRunner Runner) {
	t.Helper()
	for _, d := range Diff(t, scenario, Oracle(t), goRunner) {
		t.Error(d)
	}
}

// Diff runs the scenario with each runner in its own fresh HOME, and describes every difference in exit status, output or state files.
func Diff(t testing.TB, scenario Scenario, a, b Runner) []string {
	t.Helper()
	homeA, homeB := Home(t), Home(t)
	ra, rb := scenario(t, homeA, a), scenario(t, homeB, b)
	if len(ra) != len(rb) {
		return []string{fmt.Sprintf("the scenario gave %d results and %d results", len(ra), len(rb))}
	}
	var diffs []string
	for i := range ra {
		x, y := Normalize(ra[i], homeA), Normalize(rb[i], homeB)
		if x != y {
			diffs = append(diffs, fmt.Sprintf("step %d differs\n  first:  %+v\n  second: %+v", i, x, y))
		}
	}
	sa, sb := State(t, homeA), State(t, homeB)
	if len(sa) != len(sb) {
		diffs = append(diffs, fmt.Sprintf("state files differ: first %v, second %v", keys(sa), keys(sb)))
	}
	for name, want := range sa {
		if got, ok := sb[name]; !ok || got != want {
			diffs = append(diffs, fmt.Sprintf("state file %s differs\n  first:  %q\n  second: %q", name, want, got))
		}
	}
	return diffs
}

func keys(m map[string]string) []string {
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
