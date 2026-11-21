package main

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// cmux can be asked which process runs in which surface: `cmux top --all --processes
// --format tsv` has one row per window, workspace, pane, surface and process, each with its
// parent, so a session's process leads up to the surface that holds it.

var cmuxRefRe = regexp.MustCompile(`^(window|workspace|pane|surface):[0-9]+$`)

// cmuxPlace is where a process sits, by cmux's own refs.
type cmuxPlace struct{ Window, Workspace, Pane, Surface string }

// parseCmuxTop reads the tsv into a map from an id to its parent. Columns are cpu, memory,
// count, kind, id, parent and a label; only the kind, id and parent are used.
func parseCmuxTop(out string) map[string]string {
	parents := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 6 {
			continue
		}
		kind, id, parent := f[3], f[4], f[5]
		switch kind {
		case "process":
			if n, err := strconv.Atoi(id); err != nil || n <= 0 {
				continue
			}
		case "window", "workspace", "pane", "surface":
			if !cmuxRefRe.MatchString(id) || !strings.HasPrefix(id, kind+":") {
				continue
			}
		default:
			continue
		}
		if parent != "" && parent != "total" {
			parents[id] = parent
		}
	}
	return parents
}

// findCmuxPlace walks up from the closest process in chain that cmux knows to its surface,
// pane, workspace and window. All four must be found, and each must be a plain ref.
func findCmuxPlace(parents map[string]string, chain []int) (cmuxPlace, bool) {
	for _, pid := range chain {
		id := strconv.Itoa(pid)
		if _, ok := parents[id]; !ok {
			continue
		}
		var p cmuxPlace
		for hops := 0; hops < 16 && id != ""; hops++ {
			switch {
			case strings.HasPrefix(id, "surface:"):
				p.Surface = id
			case strings.HasPrefix(id, "pane:"):
				p.Pane = id
			case strings.HasPrefix(id, "workspace:"):
				p.Workspace = id
			case strings.HasPrefix(id, "window:"):
				p.Window = id
			}
			id = parents[id]
		}
		valid := true
		for _, ref := range []string{p.Surface, p.Pane, p.Workspace, p.Window} {
			valid = valid && cmuxRefRe.MatchString(ref) // a parent column is checked here, as the ids are in parseCmuxTop
		}
		if valid {
			return p, true
		}
	}
	return cmuxPlace{}, false
}

// cmuxSteps are the commands that bring a surface to the front, each one fixed, in order.
func cmuxSteps(p cmuxPlace) [][]string {
	return [][]string{
		{"focus-window", "--window", p.Window},
		{"select-workspace", "--workspace", p.Workspace, "--window", p.Window},
		{"focus-pane", "--pane", p.Pane, "--workspace", p.Workspace, "--window", p.Window},
		{"focus-panel", "--panel", p.Surface, "--workspace", p.Workspace, "--window", p.Window},
	}
}

// jumpCmux looks the session up in cmux and focuses its surface. ok is false when cmux does
// not know any process of the session; an error means cmux itself failed.
func jumpCmux(ctx context.Context, chains [][]int, run runner) (ok bool, err error) {
	out, err := run(ctx, "cmux", "top", "--all", "--processes", "--format", "tsv")
	if err != nil {
		return false, err
	}
	parents := parseCmuxTop(out)
	for _, chain := range chains {
		if p, found := findCmuxPlace(parents, chain); found {
			for _, step := range cmuxSteps(p) {
				if _, err := run(ctx, "cmux", step...); err != nil {
					return false, err
				}
			}
			return true, nil
		}
	}
	return false, nil
}
