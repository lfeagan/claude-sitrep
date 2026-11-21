package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// the same message a clipboard command would send, to compare against
func clip(text string) string { return fmt.Sprintf("%#v", tea.SetClipboard(text)()) }

func TestYCopiesTheIdAndShiftYTheResumeCommand(t *testing.T) {
	m := loaded(t, tabWatch)
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
	m, _ := press(t, loaded(t, tabWatch), "d")
	if _, cmd := press(t, m, "y"); cmd == nil {
		t.Errorf("y copies from the detail pane too")
	}
}

func TestCopyRefusesAnUnusualId(t *testing.T) {
	snap := loadFixture(t)
	snap.Sessions[0].ID = "abc; rm -rf /"
	m := newModel("x", time.Second, tabWatch)
	next, _ := m.Update(snapMsg{snap: snap})
	m = next.(model)
	for _, k := range []string{"y", "Y"} {
		after, cmd := press(t, m, k)
		if cmd != nil || !strings.Contains(after.statusText(), "unusual") {
			t.Errorf("%s: an id with shell characters is never copied: %q", k, after.statusText())
		}
	}
	if _, cmd := press(t, newModel("x", time.Second, tabWatch), "y"); cmd != nil {
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
