package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"fmt"
	"github.com/charmbracelet/x/ansi"
)

const previewOut = "would mark dea0 (ddead) ended\nwould ask: move 3 item(s) from dea0 (ddead) to the inbox\nwould mark 1 ended, would ask about 3 item(s)\n"

func pruneModel(t *testing.T, answers map[string]string) (model, *fake) {
	t.Helper()
	m := loaded(t, tabWatch)
	m.bin = "/x/sitrep"
	f := &fake{answer: answers}
	m.run = f.run
	return m, f
}

// run executes the command a key returned and feeds its message back.
func feed(t *testing.T, m model, cmd tea.Cmd) model {
	t.Helper()
	if cmd == nil {
		t.Fatalf("a command was expected")
	}
	next, _ := m.Update(cmd())
	return next.(model)
}

func TestPruneKeyRunsTheDryRunFirst(t *testing.T) {
	m, f := pruneModel(t, map[string]string{"/x/sitrep prune --dry-run": previewOut})
	m, cmd := press(t, m, "p")
	if m.prune != pruneLoading || cmd == nil {
		t.Fatalf("p starts a preview: phase %d", m.prune)
	}
	m = feed(t, m, cmd)
	if len(f.calls) != 1 || f.calls[0] != "/x/sitrep prune --dry-run" {
		t.Errorf("only the dry run runs first: %v", f.calls)
	}
	if m.prune != pruneConfirm || len(m.pruneText) != 3 {
		t.Fatalf("the preview is shown for confirming: phase %d, %d lines", m.prune, len(m.pruneText))
	}
	if v := m.View(); !strings.Contains(v.Content, "would mark dea0") || !strings.Contains(v.Content, "y   mark them ended") {
		t.Errorf("the screen shows the preview and the choices:\n%s", v.Content)
	}
}

func TestPruneYMovesAndNKeeps(t *testing.T) {
	for _, c := range []struct{ key, want string }{{"y", "/x/sitrep prune -y"}, {"n", "/x/sitrep prune --keep"}} {
		m, f := pruneModel(t, map[string]string{"/x/sitrep prune --dry-run": previewOut, c.want: "1 marked ended, 3 item(s) moved to the inbox\n"})
		m, cmd := press(t, m, "p")
		m = feed(t, m, cmd)
		m, cmd = press(t, m, c.key)
		if m.prune != pruneBusy || cmd == nil {
			t.Fatalf("%s starts the real prune", c.key)
		}
		m = feed(t, m, cmd)
		if got := f.calls[len(f.calls)-1]; got != c.want {
			t.Errorf("%s runs exactly %q, ran %q", c.key, c.want, got)
		}
		if m.prune != pruneNone || m.statusText() != "1 marked ended, 3 item(s) moved to the inbox" {
			t.Errorf("%s: the result is shown: phase %d, %q", c.key, m.prune, m.statusText())
		}
		if !m.fetching {
			t.Errorf("%s: the screen refreshes after a prune", c.key)
		}
	}
}

func TestPruneCancelAndIgnoredKeys(t *testing.T) {
	m, f := pruneModel(t, map[string]string{"/x/sitrep prune --dry-run": previewOut})
	m, cmd := press(t, m, "p")
	m = feed(t, m, cmd)
	calls := len(f.calls)
	for _, k := range []string{"x", "enter", "tab", "a", "j", "d", "p", "space", "1", "?", "s", "r", "Y", "N"} {
		after, c := press(t, m, k)
		if after.prune != pruneConfirm || c != nil || after.showEnded != m.showEnded || after.tab != m.tab || after.detail || after.selID != m.selID {
			t.Errorf("%q must do nothing while the question is open", k)
		}
	}
	if len(f.calls) != calls {
		t.Errorf("no command ran: %v", f.calls)
	}
	for _, k := range []string{"esc", "q"} {
		after, c := press(t, m, k)
		if after.prune != pruneNone || c != nil || len(after.pruneText) != 0 {
			t.Errorf("%s cancels without running anything", k)
		}
	}
	if _, quit := press(t, m, "ctrl+c"); quit == nil {
		t.Errorf("ctrl+c still quits")
	}
}

func TestPruneIgnoresEveryKeyWhileRunning(t *testing.T) {
	m, _ := pruneModel(t, map[string]string{"/x/sitrep prune --dry-run": previewOut})
	m, cmd := press(t, m, "p")
	m = feed(t, m, cmd)
	m, _ = press(t, m, "y")
	for _, k := range []string{"y", "n", "esc", "q", "enter", "p"} {
		after, c := press(t, m, k)
		if after.prune != pruneBusy || c != nil {
			t.Errorf("%q while the prune runs must do nothing (a held key must not start another)", k)
		}
	}
	if _, quit := press(t, m, "ctrl+c"); quit == nil {
		t.Errorf("ctrl+c quits")
	}
	loading, _ := press(t, loaded(t, tabWatch), "p")
	if after, c := press(t, loading, "y"); after.prune != pruneLoading || c != nil {
		t.Errorf("y while the preview is still running does nothing")
	}
}

func TestPruneNothingToDoAndErrors(t *testing.T) {
	m, _ := pruneModel(t, map[string]string{"/x/sitrep prune --dry-run": "would mark 0 ended, would ask about 0 item(s)\n"})
	m, cmd := press(t, m, "p")
	m = feed(t, m, cmd)
	if m.prune != pruneNone || m.statusText() != "nothing to prune" {
		t.Errorf("an empty preview needs no question: phase %d, %q", m.prune, m.statusText())
	}
	bad := &fake{fail: map[string]error{"/x/sitrep prune": errors.New("sitrep: cannot read the live-session registry")}}
	m2 := loaded(t, tabWatch)
	m2.bin, m2.run = "/x/sitrep", bad.run
	m2, cmd = press(t, m2, "p")
	m2 = feed(t, m2, cmd)
	if m2.prune != pruneNone || !strings.Contains(m2.statusText(), "cannot read the live-session registry") {
		t.Errorf("a failing preview is reported: %q", m2.statusText())
	}
}

func TestPruneGuards(t *testing.T) {
	m := loaded(t, tabWatch)
	m.readOnly = true
	if after, cmd := press(t, m, "p"); after.prune != pruneNone || cmd != nil || !strings.Contains(after.statusText(), "read-only") {
		t.Errorf("read-only turns p off: %q", after.statusText())
	}
	snap := loadFixture(t)
	snap.Registry = "unavailable"
	next, _ := loaded(t, tabWatch).Update(snapMsg{snap: snap})
	if after, cmd := press(t, next.(model), "p"); after.prune != pruneNone || cmd != nil || !strings.Contains(after.statusText(), "registry") {
		t.Errorf("without the registry p says so: %q", after.statusText())
	}
}

func TestPruneScreenSurvivesRefreshes(t *testing.T) {
	m, _ := pruneModel(t, map[string]string{"/x/sitrep prune --dry-run": previewOut})
	m, cmd := press(t, m, "p")
	m = feed(t, m, cmd)
	next, _ := m.Update(snapMsg{snap: loadFixture(t)})
	next, _ = next.Update(tickMsg(time.Now()))
	after := next.(model)
	if after.prune != pruneConfirm || len(after.pruneText) != 3 {
		t.Errorf("a refresh must not close the question or change what it shows")
	}
}

func TestPruneOutputIsCleanedAndCapped(t *testing.T) {
	hostile := "would mark x" + string(rune(0x1b)) + "[31m ended" + string(rune(0x202e)) + "\n"
	if got := pruneLines(hostile); len(got) != 1 || strings.ContainsRune(got[0], 0x1b) || strings.ContainsRune(got[0], 0x202e) {
		t.Errorf("control and format characters are stripped: %q", got)
	}
	var b strings.Builder
	for i := 0; i < 500; i++ {
		b.WriteString("would mark something ended\n")
	}
	if got := pruneLines(b.String()); len(got) != maxPruneLines {
		t.Errorf("the lines are capped: %d", len(got))
	}
	if got := pruneLines(strings.Repeat("x", 100000)); len(got) != 1 || len(got[0]) > maxPruneBytes {
		t.Errorf("the bytes are capped: %d", len(got[0]))
	}
	if !nothingToPrune(pruneLines("would mark 0 ended, would ask about 0 item(s)")) || nothingToPrune(pruneLines("would ask: move 2 item(s) from a to the inbox")) {
		t.Errorf("nothingToPrune")
	}
}

func TestPruneScreenFits(t *testing.T) {
	lines := pruneLines(previewOut + strings.Repeat("would mark a very long line "+strings.Repeat("x ", 80)+"ended\n", 3))
	for _, phase := range []int{pruneLoading, pruneConfirm, pruneBusy} {
		for w := 1; w <= 100; w += 7 {
			for h := 1; h <= 40; h += 3 {
				out := strings.Split(pruneScreen(lines, phase, w, h, plainTheme()), "\n")
				if len(out) > h {
					t.Errorf("phase %d %dx%d: %d lines", phase, w, h, len(out))
				}
				for _, l := range out {
					if ansi.StringWidth(l) > w {
						t.Errorf("phase %d %dx%d: a line is wider than the screen: %q", phase, w, h, l)
					}
				}
			}
		}
	}
	golden(t, "prune_80x14", pruneScreen(pruneLines(previewOut), pruneConfirm, 80, 14, plainTheme()))
}

func TestReadOnlyHelpHidesPrune(t *testing.T) {
	if !strings.Contains(helpScreen(80, 35, false, plainTheme()), "prune the sessions") {
		t.Errorf("the key list names p")
	}
	ro := helpScreen(80, 35, true, plainTheme())
	if strings.Contains(ro, "prune the sessions") || !strings.Contains(ro, "read-only") {
		t.Errorf("read-only hides p and says why:\n%s", ro)
	}
}

func TestTheChoicesAlwaysShow(t *testing.T) {
	var long []string
	for i := 0; i < 30; i++ {
		long = append(long, fmt.Sprintf("would mark session %d ended", i))
	}
	for h := 5; h <= 40; h++ {
		out := pruneScreen(long, pruneConfirm, 80, h, plainTheme())
		for _, want := range []string{"y   mark them ended", "n   mark them ended", "esc cancel"} {
			if !strings.Contains(out, want) {
				t.Errorf("height %d: the preview must give way, so %q shows:\n%s", h, want, out)
			}
		}
		if strings.Count(out, "\n")+1 > h {
			t.Errorf("height %d: too tall", h)
		}
	}
}
