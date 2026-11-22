package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// the same message a clipboard command would send, to compare against
func clip(text string) string { return fmt.Sprintf("%#v", tea.SetClipboard(text)()) }

func TestYCopiesTheIdAndShiftYTheResumeCommand(t *testing.T) {
	m := loaded(t, tabCards)
	id := m.selID
	m, cmd := press(t, m, "y")
	if cmd == nil || fmt.Sprintf("%#v", cmd()) != clip(id) {
		t.Errorf("y copies the session id")
	}
	if !strings.Contains(m.statusText(), "copied the session id: "+id) {
		t.Errorf("the status bar says what was copied, for a terminal that ignores the clipboard: %q", m.statusText())
	}
	m, cmd = press(t, m, "Y")
	if cmd == nil || fmt.Sprintf("%#v", cmd()) != clip("claude --resume "+id) {
		t.Errorf("Y copies the resume command")
	}
	if !strings.Contains(m.statusText(), "claude --resume "+id) {
		t.Errorf("%q", m.statusText())
	}
	if v := m.View(); !strings.Contains(v.Content, "claude --resume") {
		t.Errorf("the message is on screen")
	}
}

func TestCopyWorksInTheDetailPane(t *testing.T) {
	m, _ := press(t, loaded(t, tabCards), "v")
	if _, cmd := press(t, m, "y"); cmd == nil {
		t.Errorf("y copies from the detail pane too")
	}
}

func TestCopyRefusesAnUnusualId(t *testing.T) {
	snap := loadFixture(t)
	snap.Sessions[0].ID = "abc; rm -rf /"
	m := newModel("x", time.Second, tabCards)
	next, _ := m.Update(snapMsg{snap: snap})
	m = next.(model)
	for _, k := range []string{"y", "Y"} {
		after, cmd := press(t, m, k)
		if cmd != nil || !strings.Contains(after.statusText(), "unusual") {
			t.Errorf("%s: an id with shell characters is never copied: %q", k, after.statusText())
		}
	}
	if _, cmd := press(t, newModel("x", time.Second, tabCards), "y"); cmd != nil {
		t.Errorf("nothing selected, nothing copied")
	}
}

func TestCopyKeysAreOffWhileAQuestionIsOpen(t *testing.T) {
	m, _ := pruneModel(t, map[string]string{"/x/sitrep prune --dry-run": previewOut})
	m, cmd := press(t, m, "p")
	m = feed(t, m, cmd)
	if _, c := press(t, m, "Y"); c != nil {
		t.Errorf("Y during the prune question must do nothing")
	}
}

func ringOf(cmd tea.Cmd) string {
	if cmd == nil {
		return ""
	}
	return fmt.Sprintf("%#v", cmd())
}

func TestNewlyNeedYou(t *testing.T) {
	mk := func(sessions ...Session) Snapshot { return Snapshot{Version: 1, Sessions: sessions} }
	a := Session{ID: "a", Running: true, NeedsInput: false}
	aWaits := Session{ID: "a", Running: true, NeedsInput: true}
	b := Session{ID: "b", Running: true, NeedsInput: true}
	for name, c := range map[string]struct {
		prev, next Snapshot
		want       bool
	}{
		"a session starts waiting":               {mk(a), mk(aWaits), true},
		"one already waiting does not ring":      {mk(aWaits), mk(aWaits), false},
		"one that stopped waiting does not":      {mk(aWaits), mk(a), false},
		"a new session already waiting rings":    {mk(), mk(aWaits), true},
		"a second one starting rings":            {mk(aWaits), mk(aWaits, b), true},
		"an ended session never rings":           {mk(a), mk(Session{ID: "a", NeedsInput: true}), false},
		"a snoozed session reads as not needing": {mk(a), mk(Session{ID: "a", Running: true, Snoozed: true}), false},
		"nothing at all":                         {mk(), mk(), false},
	} {
		if got := newlyNeedYou(c.prev, c.next); got != c.want {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}

func TestTheBellRingsOnceForANewWaitingSession(t *testing.T) {
	waiting := func(snap Snapshot, id string, w bool) Snapshot {
		snap.Sessions = append([]Session(nil), snap.Sessions...)
		for i := range snap.Sessions {
			if snap.Sessions[i].Short == id {
				snap.Sessions[i].NeedsInput = w
			}
		}
		return snap
	}
	base := waiting(waiting(loadFixture(t), "b1fb", false), "15f1", false)
	m := newModel("x", time.Second, tabCards)
	next, cmd := m.Update(snapMsg{snap: waiting(base, "b1fb", true)}) // the first load: sessions already waiting
	if cmd != nil {
		t.Errorf("no bell on the first load")
	}
	m = next.(model)
	if _, cmd = m.Update(snapMsg{snap: waiting(base, "b1fb", true)}); cmd != nil {
		t.Errorf("no bell when nothing is new")
	}
	next, _ = m.Update(snapMsg{snap: base}) // it stops waiting
	m = next.(model)
	_, cmd = m.Update(snapMsg{snap: waiting(waiting(base, "b1fb", true), "15f1", true)}) // two start at once
	if ringOf(cmd) != fmt.Sprintf("%#v", tea.Raw("\a")()) {
		t.Errorf("one bell for the sessions that just began waiting")
	}
	m.bell = false
	if _, cmd = m.Update(snapMsg{snap: waiting(base, "b1fb", true)}); cmd != nil {
		t.Errorf("--no-bell silences it")
	}
}

func snoozeModel(t *testing.T, tab int) (model, *fake) {
	t.Helper()
	m := loaded(t, tab)
	m.bin = "/x/sitrep"
	f := &fake{answer: map[string]string{"/x/sitrep snooze": "snoozed 15f1 until it changes\n"}}
	m.run = f.run
	m.snap.Sessions[0].NeedsInput = true // z snoozes a session that needs you; the selected one is the first
	return m, f
}

func TestZSnoozesAndUnsnoozes(t *testing.T) {
	m, f := snoozeModel(t, tabCards)
	id := m.selID
	m, cmd := press(t, m, "z")
	if cmd == nil || !m.acting {
		t.Fatalf("z starts a snooze")
	}
	m = feed(t, m, cmd)
	if len(f.calls) != 1 || f.calls[0] != "/x/sitrep snooze "+id {
		t.Errorf("the one fixed command: %v", f.calls)
	}
	if m.acting || m.statusText() != "snoozed 15f1 until it changes" || !m.fetching {
		t.Errorf("the result is shown and the screen refreshes: %q fetching %v", m.statusText(), m.fetching)
	}
	// the refresh says it is snoozed: z now ends the snooze
	snap := loadFixture(t)
	snap.Sessions[0].Snoozed = true
	next, _ := m.Update(snapMsg{snap: snap})
	m = next.(model)
	m, cmd = press(t, m, "z")
	m = feed(t, m, cmd)
	if got := f.calls[len(f.calls)-1]; got != "/x/sitrep snooze --off "+id {
		t.Errorf("z on a snoozed session ends the snooze: %q", got)
	}
}

func TestZGuards(t *testing.T) {
	m, f := snoozeModel(t, tabCards)
	m.readOnly = true
	if after, cmd := press(t, m, "z"); cmd != nil || !strings.Contains(after.statusText(), "read-only") {
		t.Errorf("read-only turns z off: %q", after.statusText())
	}
	m.readOnly = false
	m.acting = true
	if _, cmd := press(t, m, "z"); cmd != nil {
		t.Errorf("a second z while one runs does nothing")
	}
	snap := loadFixture(t)
	snap.Sessions[0].ID = "a b; c"
	next, _ := newModel("x", time.Second, tabCards).Update(snapMsg{snap: snap})
	if after, cmd := press(t, next.(model), "z"); cmd != nil || !strings.Contains(after.statusText(), "unusual") {
		t.Errorf("an unusual id is left alone")
	}
	if _, cmd := press(t, newModel("x", time.Second, tabCards), "z"); cmd != nil {
		t.Errorf("nothing selected, nothing to snooze")
	}
	if len(f.calls) != 0 {
		t.Errorf("none of those ran a command: %v", f.calls)
	}
	bad := &fake{fail: map[string]error{"/x/sitrep snooze": errors.New("sitrep: cannot read when 15f1 last changed")}}
	m2, _ := snoozeModel(t, tabCards)
	m2.run = bad.run
	m2, cmd := press(t, m2, "z")
	m2 = feed(t, m2, cmd)
	if m2.acting || !strings.Contains(m2.statusText(), "cannot read when") {
		t.Errorf("an error is shown and the key works again: %q", m2.statusText())
	}
}

func TestSnoozedIsDrawnHollowAndWithoutPink(t *testing.T) {
	s := loadFixture(t)
	s.Sessions[2].NeedsInput, s.Sessions[2].Snoozed = false, true
	out := render(s, tabCards, "", opts(80, 40))
	if !strings.Contains(out, "○ waiting 3m") || strings.Contains(out, "● waiting") {
		t.Errorf("a snoozed session has the hollow mark:\n%s", out)
	}
	if needYou(s) != 0 {
		t.Errorf("a snoozed session is not counted as needing you: %d", needYou(s))
	}
	if got := strings.Join(detailLines(s.Sessions[2], 80), "\n"); !strings.Contains(got, "snoozed until it changes") {
		t.Errorf("the detail says so:\n%s", got)
	}
	if ro := helpScreen(80, 30, true, plainTheme()); strings.Contains(ro, "snooze the pink") {
		t.Errorf("read-only hides z from the key list")
	}
}

func TestACommandGetsTheIdNeverTheAlias(t *testing.T) {
	m := loaded(t, tabCards)
	snap := loadFixture(t)
	for i := range snap.Sessions {
		snap.Sessions[i].Alias = fmt.Sprintf("name-%d", i)
	}
	snap.Sessions[0].NeedsInput = true
	next, _ := m.Update(snapMsg{snap: snap})
	m = next.(model)
	m.bin = "/x/sitrep"
	f := &fake{answer: map[string]string{"/x/sitrep snooze": "ok\n"}}
	m.run = f.run
	id := m.selID
	m, cmd := press(t, m, "z")
	m = feed(t, m, cmd)
	if len(f.calls) != 1 || f.calls[0] != "/x/sitrep snooze "+id || strings.Contains(f.calls[0], "name-") {
		t.Errorf("calls %v, want the id %q", f.calls, id)
	}
}

func aliasModel(t *testing.T) (model, *fake) {
	t.Helper()
	m := loaded(t, tabCards)
	m.bin = "/x/sitrep"
	f := &fake{answer: map[string]string{"/x/sitrep alias": "15f1 is now called work\n"}}
	m.run = f.run
	return m, f
}

func TestNKeyNamesTheSelectedSession(t *testing.T) {
	m, f := aliasModel(t)
	id := m.selID
	m, _ = press(t, m, "n")
	if m.prompt == nil || m.prompt.verb != "alias" || m.prompt.qid != id || m.prompt.text != "" {
		t.Fatalf("n opens a prompt for the session: %+v", m.prompt)
	}
	if v := m.View().Content; !strings.Contains(v, "name for ") || !strings.Contains(v, "empty clears") {
		t.Errorf("the prompt is on the status line:\n%s", v)
	}
	m = typed(t, m, "work")
	m, cmd := press(t, m, "enter")
	if cmd == nil {
		t.Fatalf("enter runs the command")
	}
	m = feed(t, m, cmd)
	if len(f.calls) != 1 || f.calls[0] != "/x/sitrep alias "+id+" work" {
		t.Errorf("the one fixed command: %v", f.calls)
	}
	if m.prompt != nil || m.statusText() != "15f1 is now called work" || !m.fetching {
		t.Errorf("the answer is shown and the screen refreshes: %q fetching %v", m.statusText(), m.fetching)
	}
}

func TestNKeyEditsAndClearsAnExistingName(t *testing.T) {
	m, f := aliasModel(t)
	snap := loadFixture(t)
	snap.Sessions[0].Alias = "old"
	next, _ := m.Update(snapMsg{snap: snap})
	m = next.(model)
	m, _ = press(t, m, "n")
	if m.prompt == nil || m.prompt.text != "old" {
		t.Fatalf("the prompt starts with the name it has: %+v", m.prompt)
	}
	for i := 0; i < 3; i++ {
		m, _ = press(t, m, "backspace")
	}
	m, cmd := press(t, m, "enter")
	m = feed(t, m, cmd)
	if len(f.calls) != 1 || f.calls[0] != "/x/sitrep alias "+snap.Sessions[0].ID+" --off" {
		t.Errorf("an empty name clears it: %v", f.calls)
	}
}

func TestNKeyRefusals(t *testing.T) {
	// nothing to clear
	m, f := aliasModel(t)
	m, _ = press(t, m, "n")
	m, cmd := press(t, m, "enter")
	if cmd != nil || len(f.calls) != 0 || !strings.Contains(m.statusText(), "nothing to set") {
		t.Errorf("empty with no name: %v %q", f.calls, m.statusText())
	}
	// a name that the grammar refuses is refused here, with the whole rule, and no command runs
	for _, bad := range []string{"two words", "Frontend", "bad!", "9lives", "inbox", strings.Repeat("a", 33)} {
		m, _ = press(t, m, "n")
		m = typed(t, m, bad)
		m, cmd = press(t, m, "enter")
		if cmd != nil || len(f.calls) != 0 || !strings.Contains(m.statusText(), "lowercase letters, digits and hyphens") {
			t.Errorf("%q: %v %q", bad, f.calls, m.statusText())
		}
	}
	// esc cancels
	m, _ = press(t, m, "n")
	m = typed(t, m, "abc")
	m, cmd = press(t, m, "esc")
	if m.prompt != nil || cmd != nil || len(f.calls) != 0 {
		t.Errorf("esc: %+v %v", m.prompt, f.calls)
	}
	// read-only
	m, f = aliasModel(t)
	m.readOnly = true
	m, _ = press(t, m, "n")
	if m.prompt != nil || !strings.Contains(m.statusText(), "read-only") || len(f.calls) != 0 {
		t.Errorf("read-only: %+v %q", m.prompt, m.statusText())
	}
}

func TestNKeyWorksOnAnEndedSession(t *testing.T) {
	m, f := aliasModel(t)
	m.showEnded = true
	snap := loadFixture(t)
	var ended string
	for _, x := range snap.Sessions {
		if !x.Running {
			ended = x.ID
		}
	}
	if ended == "" {
		t.Skip("the fixture has no ended session")
	}
	m.selID = ended
	m, _ = press(t, m, "n")
	m = typed(t, m, "old-one")
	m, cmd := press(t, m, "enter")
	m = feed(t, m, cmd)
	if len(f.calls) != 1 || f.calls[0] != "/x/sitrep alias "+ended+" old-one" {
		t.Errorf("calls %v", f.calls)
	}
}

func TestZOnASessionThatDoesNotNeedYouSaysSoAndRunsNothing(t *testing.T) {
	m, f := snoozeModel(t, tabCards)
	m.snap.Sessions[0].NeedsInput = false // a working session: a snooze would mute nothing
	m, cmd := press(t, m, "z")
	if cmd != nil || m.acting || len(f.calls) != 0 {
		t.Fatalf("z does not run a command for it: %v", f.calls)
	}
	if got := m.statusText(); !strings.Contains(got, "does not need you") || !strings.Contains(got, "nothing to snooze") {
		t.Errorf("the status says why: %q", got)
	}
	m.snap.Sessions[0].Snoozed = true // a snoozed one can still be un-snoozed
	if _, cmd := press(t, m, "z"); cmd == nil {
		t.Error("z on a snoozed session ends the snooze")
	}
}
