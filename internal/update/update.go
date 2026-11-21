// Package update tells a user that a newer release exists. Once a day a detached child asks the repository for its version tags
// (`git ls-remote`) and writes the newest to the state folder; the status line only reads that file. Nothing is downloaded or installed.
package update

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/runner"
	"github.com/lfeagan/claude-sitrep/internal/state"
)

const (
	Repo     = "https://github.com/lfeagan/claude-sitrep.git"
	Interval = 24 * time.Hour
	Deadline = 20 * time.Second
	file     = "update"
)

// Version is a release number. It is made only by Parse, so what is shown has the form MAJOR.MINOR.PATCH and nothing else.
type Version [3]int

// Parse reads vMAJOR.MINOR.PATCH (the v is optional); each part is at most nine digits. Other tags, such as bash-final, are not versions.
func Parse(s string) (Version, bool) {
	parts := strings.Split(strings.TrimPrefix(s, "v"), ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	var v Version
	for i, p := range parts {
		if p == "" || len(p) > 9 {
			return Version{}, false
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				return Version{}, false
			}
		}
		v[i], _ = strconv.Atoi(p)
	}
	return v, true
}

func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }

// Newer says whether v is a later release than o.
func (v Version) Newer(o Version) bool {
	for i := range v {
		if v[i] != o[i] {
			return v[i] > o[i]
		}
	}
	return false
}

// Newest is the highest version among the tags of `git ls-remote --tags --refs` output.
func Newest(out []byte) (Version, bool) {
	var best Version
	found := false
	for _, line := range bytes.Split(out, []byte("\n")) {
		_, ref, ok := strings.Cut(string(line), "\trefs/tags/")
		if !ok || !strings.HasPrefix(ref, "v") {
			continue
		}
		if v, ok := Parse(ref); ok && (!found || v.Newer(best)) {
			best, found = v, true
		}
	}
	return best, found
}

// Record is what the last check left: when it ran, and the newest release it saw (the zero Version when it saw none).
type Record struct {
	Checked int64
	Latest  Version
}

func path(st state.Store) string { return filepath.Join(st.Dir, file) }

// Read returns the record; a missing or broken file is a record of no check.
func Read(st state.Store) Record {
	f := strings.Fields(state.FirstLine(path(st)))
	var r Record
	if len(f) == 2 {
		r.Checked, _ = strconv.ParseInt(f[0], 10, 64)
		r.Latest, _ = Parse(f[1])
	}
	return r
}

func write(st state.Store, r Record) error {
	return st.WriteFile(path(st), []byte(strconv.FormatInt(r.Checked, 10)+" "+r.Latest.String()+"\n"))
}

// Claim says whether a check is due, and if so records the attempt now, keeping what is known, so that a failing check is not retried
// before the next interval. Two hooks racing may both claim; the cost is one more `git ls-remote`.
func Claim(st state.Store, now time.Time) bool {
	r := Read(st)
	if c := time.Unix(r.Checked, 0); !now.Before(c) && now.Sub(c) < Interval {
		return false
	}
	r.Checked = now.Unix()
	return write(st, r) == nil
}

// Check asks the repository for its newest release and records it. A failure leaves the record as the claim made it.
func Check(ctx context.Context, run runner.Runner, st state.Store, now time.Time, repo string, env []string) {
	ctx, cancel := context.WithTimeout(ctx, Deadline)
	defer cancel()
	out, err := run.Run(ctx, runner.Cmd{Name: "git", Args: []string{"ls-remote", "--tags", "--refs", repo}, Env: env, NewGroup: true})
	if err != nil || ctx.Err() != nil {
		return
	}
	if v, ok := Newest(out); ok {
		_ = write(st, Record{Checked: now.Unix(), Latest: v})
	}
}

// Notice is the line for the status line, or empty when the program is up to date or no check has found a newer release.
func Notice(st state.Store, current string) string {
	cur, ok := Parse(current)
	if !ok {
		return ""
	}
	if l := Read(st).Latest; l.Newer(cur) {
		return fmt.Sprintf("Update: sitrep %s is out (you have %s); see the README, Updating", l, cur)
	}
	return ""
}
