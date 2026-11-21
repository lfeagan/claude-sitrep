package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const (
	item1 = "look at the nightly job"
	item2 = "check the CI flake on macOS and write down what the runner image does about the node deprecation"
)

func inboxModel(t *testing.T) (model, *fake) {
	t.Helper()
	m := loaded(t, tabWatch)
	m.bin = "/x/sitrep"
	f := &fake{answer: map[string]string{"/x/sitrep move": "moved to 15f1: " + item1 + "\n"}}
	m.run = f.run
	m, _ = press(t, m, "i")
	return m, f
}

func TestInboxOpensAndCloses(t *testing.T) {
	m, _ := inboxModel(t)
	if !m.inbox || m.inboxTo != m.selID || m.inboxIndex() != 0 {
		t.Fatalf("i opens the inbox on the first item, aimed at the selected session: %+v", m)
	}
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "down")
	if m.inboxIndex() != 1 {
		t.Errorf("selection stops at the last item: %d", m.inboxIndex())
	}
	m, _ = press(t, m, "home")
	m, _ = press(t, m, "up")
	if m.inboxIndex() != 0 {
		t.Errorf("and at the first: %d", m.inboxIndex())
	}
	m, _ = press(t, m, "end")
	if m.inboxIndex() != 1 {
		t.Errorf("end selects the last: %d", m.inboxIndex())
	}
	for _, k := range []string{"esc", "i"} {
		m2, _ := inboxModel(t)
		m2, _ = press(t, m2, k)
		if m2.inbox {
			t.Errorf("%s closes it", k)
		}
	}
	if _, cmd := press(t, m, "q"); cmd == nil {
		t.Error("q quits")
	}
}

func TestInboxTakeAsksThenMovesTheItemItShowed(t *testing.T) {
	m, f := inboxModel(t)
	id := m.inboxTo
	m, cmd := press(t, m, "t")
	if !m.asking || cmd != nil || len(f.calls) != 0 {
		t.Fatalf("t asks and runs nothing: %+v %v", m.asking, f.calls)
	}
	m, cmd = press(t, m, "y")
	if cmd == nil || !m.acting || m.asking {
		t.Fatalf("y runs the move and ends the question: acting %v asking %v", m.acting, m.asking)
	}
	if _, again := press(t, m, "y"); again != nil {
		t.Error("a second y does nothing while the move runs")
	}
	if _, again := press(t, m, "t"); again != nil {
		t.Error("so does t")
	}
	m = feed(t, m, cmd)
	if want := "/x/sitrep move inbox " + id + " 1 --item " + item1; len(f.calls) != 1 || f.calls[0] != want {
		t.Errorf("the one fixed command, with the item's text: %v", f.calls)
	}
	if m.acting || !m.fetching || m.statusText() != "moved to 15f1: "+item1 {
		t.Errorf("the result is shown and the screen refreshes: %q fetching %v", m.statusText(), m.fetching)
	}
}

func TestInboxTakeCancelAndErrors(t *testing.T) {
	for _, k := range []string{"n", "esc", "q"} {
		m, f := inboxModel(t)
		m, _ = press(t, m, "t")
		m, cmd := press(t, m, k)
		if m.asking || cmd != nil || len(f.calls) != 0 || !m.inbox {
			t.Errorf("%s cancels the question and nothing runs", k)
		}
	}
	m, f := inboxModel(t)
	f.fail = map[string]error{"/x/sitrep move": errors.New("queue changed; run 'sitrep ls inbox'")}
	m, _ = press(t, m, "t")
	m, cmd := press(t, m, "y")
	m = feed(t, m, cmd)
	if m.acting || m.statusText() != "take: queue changed; run 'sitrep ls inbox'" {
		t.Errorf("a refusal is shown and the move is over: %q", m.statusText())
	}
	m, _ = press(t, m, "t")
	if !m.asking {
		t.Error("and the user can try again")
	}
}

func TestInboxTakeRefusals(t *testing.T) {
	cases := map[string]func(m *model){
		"the dashboard is read-only":          func(m *model) { m.readOnly = true },
		"the inbox is empty":                  func(m *model) { m.snap.Inbox = Queue{} },
		"there is no session to take it into": func(m *model) { m.inboxTo = "gone" },
		"has ended":                           func(m *model) { m.snap.Sessions[0].Running = false },
		"this session's id looks unusual":     func(m *model) { m.snap.Sessions[0].ID = "a b"; m.inboxTo = "a b" },
		"busy":                                func(m *model) { m.acting = true },
	}
	for want, set := range cases {
		m, f := inboxModel(t)
		set(&m)
		m, cmd := press(t, m, "t")
		if m.asking || cmd != nil || len(f.calls) != 0 || !strings.Contains(m.statusText(), want) {
			t.Errorf("%s: asking %v, status %q", want, m.asking, m.statusText())
		}
	}
}

func TestInboxQuestionIsDroppedWhenTheInboxOrTheTargetChanges(t *testing.T) {
	ask := func() model {
		m, _ := inboxModel(t)
		m, _ = press(t, m, "down")
		m, _ = press(t, m, "t")
		if !m.asking || m.askN != 2 || m.askItem != item2 {
			t.Fatalf("the question holds the item and its position: %d %q", m.askN, m.askItem)
		}
		return m
	}
	refresh := func(m model, edit func(*Snapshot)) model {
		s := loadFixture(t)
		edit(&s)
		next, _ := m.Update(snapMsg{snap: s})
		return next.(model)
	}
	if m := refresh(ask(), func(s *Snapshot) {}); !m.asking {
		t.Error("a refresh that changes nothing keeps the question")
	}
	m := refresh(ask(), func(s *Snapshot) { s.Inbox.Items = []string{"new first", item2} })
	if !m.asking || m.askN != 2 {
		t.Error("the same item at the same position keeps it")
	}
	m = refresh(ask(), func(s *Snapshot) { s.Inbox.Items = []string{item2, item1} })
	if m.asking || !strings.Contains(m.statusText(), "the inbox changed") {
		t.Errorf("a different item at that position drops it: %q", m.statusText())
	}
	m = refresh(ask(), func(s *Snapshot) { s.Sessions[0].Running = false })
	if m.asking || !strings.Contains(m.statusText(), "the session is gone") {
		t.Errorf("an ended target drops it: %q", m.statusText())
	}
	m = refresh(ask(), func(s *Snapshot) { s.Sessions = s.Sessions[1:] })
	if m.asking {
		t.Error("a missing target drops it")
	}
}

func TestInboxSelectionFollowsItsItem(t *testing.T) {
	m, _ := inboxModel(t)
	m, _ = press(t, m, "down")
	s := loadFixture(t)
	s.Inbox.Items = []string{"added above", item1, item2}
	s.Inbox.Count = 3
	next, _ := m.Update(snapMsg{snap: s})
	m = next.(model)
	if m.inboxIndex() != 2 {
		t.Errorf("the selection stays on its item when one is added above: %d", m.inboxIndex())
	}
	s.Inbox.Items = []string{item1}
	s.Inbox.Count = 1
	next, _ = m.Update(snapMsg{snap: s})
	if m = next.(model); m.inboxIndex() != 0 {
		t.Errorf("and is clamped when its item is gone: %d", m.inboxIndex())
	}
}

func TestInboxKeyInOtherModes(t *testing.T) {
	m := loaded(t, tabWatch)
	m, _ = press(t, m, "/")
	m, _ = press(t, m, "i")
	if m.inbox || m.filter != "i" {
		t.Errorf("i is text while filtering: inbox %v filter %q", m.inbox, m.filter)
	}
	m = loaded(t, tabWatch)
	m, _ = press(t, m, "?")
	m, _ = press(t, m, "i")
	if m.inbox || m.help {
		t.Error("i in the key list only closes it")
	}
	m, _ = press(t, m, "d")
	if m, _ = press(t, m, "i"); m.inbox || !m.detail {
		t.Error("i does nothing in the detail pane")
	}
	empty := newModel("x", 0, tabWatch)
	if empty, _ = press(t, empty, "i"); !empty.inbox {
		t.Error("the inbox opens before any data and with no session")
	}
	if out := empty.View().Content; !strings.Contains(out, "the inbox is empty") {
		t.Errorf("and says it is empty:\n%s", out)
	}
}

func TestInboxScreens(t *testing.T) {
	items := []string{item1, item2, clean("evil \x1b[31mred\u202e bidi\ttab"), "日本語の項目: データベースの性能を調べる"}
	to := &Session{Short: "15f1", Dir: "claude-sitrep"}
	th := plainTheme()
	golden(t, "inbox_80x24", inboxScreen(items, 4, 1, to, false, 0, "", false, 80, 24, th))
	golden(t, "inbox_ask_30x8", inboxScreen(items, 4, 1, to, true, 2, "", false, 30, 8, th))
	golden(t, "inbox_none_80x6", inboxScreen(nil, 0, 0, to, false, 0, "", false, 80, 6, th))
	if out := inboxScreen(items, 4, 0, to, false, 0, "", true, 80, 24, th); strings.Contains(out, "take") {
		t.Errorf("read-only does not offer t:\n%s", out)
	}
	if out := inboxScreen(items, 4, 0, to, false, 0, "take: busy", false, 80, 24, th); !strings.Contains(out, "take: busy") {
		t.Errorf("a message replaces the key hint:\n%s", out)
	}
	if out := inboxScreen(items, 4, 0, to, false, 0, "", false, 80, 24, th); !strings.Contains(out, "the first 4 of 4") && !strings.Contains(out, "4 items") {
		t.Errorf("the title counts the items:\n%s", out)
	}
	for _, h := range []int{1, 2, 3, 5} {
		for _, w := range []int{1, 2, 10, 20} {
			out := inboxScreen(items, 9, 3, to, true, 4, "", false, w, h, th)
			if n := len(strings.Split(out, "\n")); n > h {
				t.Errorf("%dx%d: %d lines", w, h, n)
			}
			for _, l := range strings.Split(out, "\n") {
				if ansi.StringWidth(l) > w {
					t.Errorf("%dx%d: a line is wider than the screen: %q", w, h, l)
				}
			}
		}
	}
}

func TestHelpHidesTakeWhenReadOnly(t *testing.T) {
	if h := helpScreen(100, 40, false, plainTheme()); !strings.Contains(h, "take the item") {
		t.Error("the key list names t")
	}
	if h := helpScreen(100, 40, true, plainTheme()); strings.Contains(h, "take the item") || !strings.Contains(h, "the inbox") {
		t.Error("read-only hides t but keeps i")
	}
}
