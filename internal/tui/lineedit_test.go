package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDropWord(t *testing.T) {
	for in, want := range map[string]string{"": "", "one": "", "one two": "one ", "one two  ": "one ", "  ": "", "a b c": "a b "} {
		if got := dropWord(in); got != want {
			t.Errorf("dropWord(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCtrlUAndCtrlWEditTheFilter(t *testing.T) {
	m := loaded(t, tabCards)
	m, _ = press(t, m, "/")
	m = typed(t, m, "web app")
	if m, _ = press(t, m, "ctrl+w"); m.filter != "web " || !m.filtering {
		t.Errorf("ctrl+w drops the last word and stays in the filter: %q filtering %v", m.filter, m.filtering)
	}
	if m, _ = press(t, m, "ctrl+u"); m.filter != "" || !m.filtering {
		t.Errorf("ctrl+u clears the line and stays in the filter: %q filtering %v", m.filter, m.filtering)
	}
	if m.selID == "" {
		t.Error("a selection is kept when the filter is cleared")
	}
}

func TestCtrlUAndCtrlWEditAPrompt(t *testing.T) {
	m := loaded(t, tabCards)
	m, _ = press(t, m, "n")
	if m.prompt == nil {
		t.Fatal("n opens the name prompt")
	}
	m = typed(t, m, "my session")
	if m, _ = press(t, m, "ctrl+w"); m.prompt == nil || m.prompt.text != "my " {
		t.Errorf("ctrl+w: %+v", m.prompt)
	}
	if m, _ = press(t, m, "ctrl+u"); m.prompt == nil || m.prompt.text != "" {
		t.Errorf("ctrl+u: %+v", m.prompt)
	}
}

func TestFilterFooterListsTheKeysThatWork(t *testing.T) {
	m := loaded(t, tabCards)
	m.width, m.height = 110, 30
	if v := m.View().Content; !strings.Contains(v, "n name") || strings.Contains(v, "enter keep") {
		t.Error("the normal footer lists the normal keys")
	}
	m, _ = press(t, m, "/")
	v := m.View().Content
	if !strings.Contains(v, "enter keep") || !strings.Contains(v, "esc cancel") || strings.Contains(v, "n name") || strings.Contains(v, "q quit") {
		t.Errorf("while typing, the footer lists the keys of the filter:\n%s", v)
	}
	m, _ = press(t, m, "enter")
	if v := m.View().Content; strings.Contains(v, "enter keep") {
		t.Error("the filter footer goes when the typing ends")
	}
}

func TestDetailFooterShowsJumpSnoozeAndCopy(t *testing.T) {
	m := loaded(t, tabCards)
	m.width, m.height = 140, 30
	m, _ = press(t, m, "v")
	v := m.View().Content
	for _, want := range []string{"enter jump", "z snooze", "y copy id", "e queue"} {
		if !strings.Contains(v, want) {
			t.Errorf("the detail footer should say %q:\n%s", want, v)
		}
	}
	m.readOnly = true
	if v := m.View().Content; strings.Contains(v, "z snooze") || !strings.Contains(v, "enter jump") {
		t.Errorf("read-only drops z and keeps the rest:\n%s", v)
	}
}

func TestQueueFooterNamesTheOtherQueue(t *testing.T) {
	m, _ := queueModel(t, qSession)
	m.width, m.height = 140, 30
	if v := m.View().Content; !strings.Contains(v, "i inbox") {
		t.Errorf("a session queue says i goes to the inbox:\n%s", v)
	}
	m, _ = queueModel(t, qInbox)
	m.width, m.height = 140, 30
	if v := m.View().Content; !strings.Contains(v, "e session queue") {
		t.Errorf("the inbox says e goes to the session's queue:\n%s", v)
	}
}

func TestSwitchingQueuesKeepsTheDetailViewUnderneath(t *testing.T) {
	m := loaded(t, tabCards)
	m, _ = press(t, m, "v")
	id := m.selID
	m, _ = press(t, m, "e")
	m, _ = press(t, m, "i")
	if !m.queue || m.qKind != qInbox || !m.detail || m.qTo != id {
		t.Errorf("i switches to the inbox: queue %v kind %d detail %v to %q", m.queue, m.qKind, m.detail, m.qTo)
	}
	m, _ = press(t, m, "e")
	if !m.queue || m.qKind != qSession || !m.detail || m.qTo != id {
		t.Errorf("e switches back: queue %v kind %d detail %v to %q", m.queue, m.qKind, m.detail, m.qTo)
	}
	m, _ = press(t, m, "e")
	if m.queue || !m.detail {
		t.Error("the key of the open queue closes it, back to the detail view")
	}
}

func namedModel(t *testing.T, name string) model {
	m, _ := aliasModel(t)
	for i := range m.snap.Sessions {
		if m.snap.Sessions[i].ID == m.selID {
			m.snap.Sessions[i].Alias = name
		}
	}
	m, _ = press(t, m, "n")
	if m.prompt == nil || m.prompt.text != name {
		t.Fatalf("the prompt opens with the current name %q: %+v", name, m.prompt)
	}
	return m
}

func TestTypingReplacesTheCurrentNameInThePrompt(t *testing.T) {
	m := typed(t, namedModel(t, "api"), "web")
	if m.prompt.text != "web" {
		t.Errorf("the first typed character replaces the name: %q", m.prompt.text)
	}
	m = typed(t, m, "2")
	if m.prompt.text != "web2" {
		t.Errorf("after that, typing appends: %q", m.prompt.text)
	}
}

func TestSpaceAndPasteReplaceTheCurrentNameToo(t *testing.T) {
	m, _ := press(t, namedModel(t, "api"), "space")
	if m.prompt.text != " " {
		t.Errorf("space replaces it: %q", m.prompt.text)
	}
	next, _ := namedModel(t, "api").Update(tea.PasteMsg{Content: "pasted"})
	if got := next.(model).prompt.text; got != "pasted" {
		t.Errorf("a paste replaces it: %q", got)
	}
}

func TestEditingKeysKeepTheCurrentNameToEdit(t *testing.T) {
	m, _ := press(t, namedModel(t, "api"), "backspace")
	m = typed(t, m, "x")
	if m.prompt.text != "apx" {
		t.Errorf("backspace edits the name, and what is typed after it is added: %q", m.prompt.text)
	}
	m, _ = press(t, namedModel(t, "api"), "ctrl+w")
	m = typed(t, m, "z")
	if m.prompt.text != "z" {
		t.Errorf("ctrl+w drops the word: %q", m.prompt.text)
	}
	m, _ = press(t, namedModel(t, "api"), "enter")
	if m.prompt != nil {
		t.Error("enter alone keeps the name and closes the prompt")
	}
}

func TestNoCurrentNameMeansTypingAppends(t *testing.T) {
	m, _ := aliasModel(t)
	m, _ = press(t, m, "n")
	m = typed(t, m, "ab")
	if m.prompt.text != "ab" {
		t.Errorf("%q", m.prompt.text)
	}
}
