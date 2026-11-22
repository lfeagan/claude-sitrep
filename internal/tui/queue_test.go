package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	item1 = "look at the nightly job"
	item2 = "check the CI flake on macOS and write down what the runner image does about the node deprecation"
	sq1   = "Decide whether to add a filter key"
	sq2   = "Add a help overlay on ? that lists the keys and what each tab shows"
	sq3   = "Write the README section for the Go dashboard"
)

// queueModel opens the inbox (kind qInbox) or the queue of the first session (qSession).
func queueModel(t *testing.T, kind int) (model, *fake) {
	t.Helper()
	m := loaded(t, tabCards)
	m.selID = m.snap.Sessions[0].ID // the session with the long queue; Cards starts with the one that needs you
	m.bin = "/x/sitrep"
	f := &fake{answer: map[string]string{"/x/sitrep": "done\n"}}
	m.run = f.run
	k := "i"
	if kind == qSession {
		k = "e"
	}
	m, _ = press(t, m, k)
	return m, f
}

// settled delivers the snapshot that follows a change, which edit keys wait for.
func settled(t *testing.T, m model) model {
	t.Helper()
	next, _ := m.Update(snapMsg{snap: loadFixture(t)})
	return next.(model)
}

func TestQueueOpensAndCloses(t *testing.T) {
	m, _ := queueModel(t, qInbox)
	if !m.queue || m.qKind != qInbox || m.qTo != m.selID || m.queueIndex() != 0 {
		t.Fatalf("i opens the inbox on its first item, aimed at the selected session: %+v", m)
	}
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "down")
	if m.queueIndex() != 1 {
		t.Errorf("selection stops at the last item: %d", m.queueIndex())
	}
	m, _ = press(t, m, "home")
	m, _ = press(t, m, "up")
	if m.queueIndex() != 0 {
		t.Errorf("and at the first: %d", m.queueIndex())
	}
	m, _ = press(t, m, "end")
	if m.queueIndex() != 1 {
		t.Errorf("end selects the last: %d", m.queueIndex())
	}
	for _, c := range []struct {
		kind int
		k    string
	}{{qInbox, "esc"}, {qInbox, "i"}, {qInbox, "backspace"}, {qSession, "esc"}, {qSession, "e"}} {
		m2, _ := queueModel(t, c.kind)
		if m2, _ = press(t, m2, c.k); m2.queue {
			t.Errorf("%s closes kind %d", c.k, c.kind)
		}
	}
	if m2, _ := queueModel(t, qSession); true {
		if m2, _ = press(t, m2, "i"); !m2.queue || m2.qKind != qInbox {
			t.Error("i inside a session queue switches to the inbox")
		}
		if m2, _ = press(t, m2, "e"); !m2.queue || m2.qKind != qSession {
			t.Error("e inside the inbox switches to the session's queue")
		}
	}
	if back, cmd := press(t, m, "q"); cmd != nil || back.queue {
		t.Errorf("q goes back from the queue and does not quit: queue %v", back.queue)
	}
	if _, cmd := press(t, m, "ctrl+c"); cmd == nil {
		t.Error("ctrl+c quits from the queue")
	}
	ms, _ := queueModel(t, qSession)
	if got := ms.queueItems(); len(got) != 3 || got[0] != sq1 {
		t.Errorf("e shows the selected session's queue: %v", got)
	}
}

func TestQueueNeedsASessionToOpen(t *testing.T) {
	m := newModel("x", 0, tabCards)
	m, _ = press(t, m, "e")
	if m.queue || !strings.Contains(m.statusText(), "no session is selected") {
		t.Errorf("e with nothing selected says so: %q", m.statusText())
	}
	m, _ = press(t, m, "i")
	if !m.queue {
		t.Error("the inbox opens before any data")
	}
	if out := m.View().Content; !strings.Contains(out, "the inbox is empty") {
		t.Errorf("and says it is empty:\n%s", out)
	}
}

func TestQueueTakeAsksThenMovesTheItemItShowed(t *testing.T) {
	m, f := queueModel(t, qInbox)
	id := m.qTo
	m, cmd := press(t, m, "t")
	if m.ask == nil || cmd != nil || len(f.calls) != 0 {
		t.Fatalf("t asks and runs nothing: %+v %v", m.ask, f.calls)
	}
	m, cmd = press(t, m, "y")
	if cmd == nil || !m.acting || m.ask != nil {
		t.Fatalf("y runs the move and ends the question: acting %v", m.acting)
	}
	for _, k := range []string{"y", "t", "x", "K", "a"} {
		if m2, again := press(t, m, k); again != nil || m2.prompt != nil || m2.ask != nil {
			t.Errorf("%s does nothing while a change runs", k)
		}
	}
	m = feed(t, m, cmd)
	if want := "/x/sitrep move inbox " + id + " 1 --item " + item1; len(f.calls) != 1 || f.calls[0] != want {
		t.Errorf("the one fixed command, with the item's text: %v", f.calls)
	}
	if m.acting || !m.fetching || m.statusText() != "done" {
		t.Errorf("the result is shown and the screen refreshes: %q fetching %v", m.statusText(), m.fetching)
	}
	if m.qText != "" || m.qSel != 0 {
		t.Errorf("the old text does not pull the selection to a twin: %q", m.qText)
	}
	next, _ := m.Update(snapMsg{snap: loadFixture(t)})
	if m = next.(model); m.settling {
		t.Error("the fresh snapshot ends the wait")
	}
}

func TestQueueRemoveAsks(t *testing.T) {
	m, f := queueModel(t, qSession)
	id := m.qTo
	m, _ = press(t, m, "down")
	m, _ = press(t, m, "x")
	if m.ask == nil || len(f.calls) != 0 || m.ask.item != sq2 {
		t.Fatalf("x asks first: %+v", m.ask)
	}
	if out := m.View().Content; !strings.Contains(out, "y/n  remove item 2?") {
		t.Errorf("the question is on screen:\n%s", out)
	}
	m, cmd := press(t, m, "y")
	m = feed(t, m, cmd)
	if want := "/x/sitrep rm " + id + " 2 --item " + sq2; len(f.calls) != 1 || f.calls[0] != want {
		t.Errorf("the one fixed command: %v", f.calls)
	}
	if m.qSel != 1 {
		t.Errorf("the selection stays at the position: %d", m.qSel)
	}
	for _, k := range []string{"n", "esc", "q"} {
		m2, f2 := queueModel(t, qSession)
		m2, _ = press(t, m2, "x")
		m2, cmd := press(t, m2, k)
		if m2.ask != nil || cmd != nil || len(f2.calls) != 0 || !m2.queue {
			t.Errorf("%s cancels the question and nothing runs", k)
		}
	}
}

func TestQueueFailureIsShownAndCanBeRetried(t *testing.T) {
	m, f := queueModel(t, qInbox)
	f.fail = map[string]error{"/x/sitrep": errors.New("queue changed; run 'sitrep ls inbox'")}
	m, _ = press(t, m, "t")
	m, cmd := press(t, m, "y")
	m = feed(t, m, cmd)
	if m.acting || m.statusText() != "take: queue changed; run 'sitrep ls inbox'" || m.qSel != 0 {
		t.Errorf("a refusal is shown and the move is over: %q", m.statusText())
	}
	if m, _ = press(t, settled(t, m), "t"); m.ask == nil {
		t.Error("and the user can try again")
	}
}

func TestQueueMovesRunFixedCommandsAndRefuseWhatCannotHappen(t *testing.T) {
	m, f := queueModel(t, qSession)
	id := m.qTo
	m, _ = press(t, m, "down")
	for k, want := range map[string]string{
		"K": "/x/sitrep reorder " + id + " 2 1 --item " + sq2,
		"J": "/x/sitrep reorder " + id + " 2 3 --item " + sq2,
		"T": "/x/sitrep top " + id + " 2 --item " + sq2,
	} {
		f.calls = nil
		m2, cmd := press(t, m, k)
		m2 = feed(t, m2, cmd)
		if len(f.calls) != 1 || f.calls[0] != want {
			t.Errorf("%s: %v", k, f.calls)
		}
		if m2.acting || !m2.fetching {
			t.Errorf("%s: the screen refreshes", k)
		}
	}
	// the selection follows the item to where it went
	for k, want := range map[string]int{"K": 0, "J": 2, "T": 0} {
		m2, cmd := press(t, m, k)
		if m2 = feed(t, m2, cmd); m2.qSel != want || m2.qText != "" {
			t.Errorf("%s selects position %d, text not yet known: %d %q", k, want, m2.qSel, m2.qText)
		}
	}
	f.calls = nil
	first, _ := press(t, m, "home")
	for _, k := range []string{"K", "T"} {
		m2, cmd := press(t, first, k)
		if cmd != nil || !strings.Contains(m2.statusText(), "already first") {
			t.Errorf("%s on the first item: %q", k, m2.statusText())
		}
	}
	last, _ := press(t, m, "end")
	if m2, cmd := press(t, last, "J"); cmd != nil || !strings.Contains(m2.statusText(), "past the items shown") {
		t.Errorf("J on the last shown item of a longer queue: %q", m2.statusText())
	}
	last.snap.Sessions[0].Queue.Count = 3
	if m2, cmd := press(t, last, "J"); cmd != nil || !strings.Contains(m2.statusText(), "already last") {
		t.Errorf("J on the last item: %q", m2.statusText())
	}
	if len(f.calls) != 0 {
		t.Errorf("none of those ran anything: %v", f.calls)
	}
}

func TestQueueAChangeDropsAnOlderSnapshot(t *testing.T) {
	m, _ := queueModel(t, qSession)
	m, cmd := press(t, m, "T")
	if cmd != nil {
		t.Skip("the first item cannot go to the front")
	}
	m, _ = press(t, m, "down")
	m, cmd = press(t, m, "T")
	m.fetching = true // a fetch that began before the change is still running
	m = feed(t, m, cmd)
	if _, again := press(t, m, "K"); again != nil || m.statusText() == "" {
		t.Error("edit keys wait for the fresh snapshot")
	}
	if m2, _ := press(t, m, "K"); !strings.Contains(m2.statusText(), "refreshing") {
		t.Errorf("and say why: %q", m2.statusText())
	}
	if !m.refetch {
		t.Fatal("the change notes that the running fetch is stale")
	}
	started := m.started
	next, cmd := m.Update(snapMsg{snap: loadFixture(t)})
	m = next.(model)
	if cmd == nil || m.refetch || !m.fetching || m.started != started+1 {
		t.Errorf("the stale snapshot is dropped and a new fetch starts: %v", m.started)
	}
}

func TestQueuePromptAddsAndPins(t *testing.T) {
	m, f := queueModel(t, qSession)
	id := m.qTo
	m.msg, m.msgAt = "old message", m.now()
	m, _ = press(t, m, "a")
	if m.prompt == nil || m.msg != "" {
		t.Fatal("a opens a prompt and clears an older message")
	}
	m = typed(t, m, "qxi go on")
	if m.prompt == nil || !m.queue || m.prompt.text != "qxi go on" {
		t.Errorf("command letters are text while typing: %+v", m.prompt)
	}
	m, _ = press(t, m, "backspace")
	if out := m.View().Content; !strings.Contains(out, "add: qxi go o▏") {
		t.Errorf("the line is on the footer:\n%s", out)
	}
	if _, cmd := press(t, m, "ctrl+c"); cmd == nil {
		t.Error("ctrl+c still quits")
	}
	m, cmd := press(t, m, "enter")
	m = feed(t, m, cmd)
	if want := "/x/sitrep add " + id + " qxi go o"; len(f.calls) != 1 || f.calls[0] != want {
		t.Errorf("add runs with the text: %v", f.calls)
	}
	m, _ = press(t, settled(t, m), "P")
	m = typed(t, m, "-x --item")
	m, cmd = press(t, m, "enter")
	m = feed(t, m, cmd)
	if want := "/x/sitrep pin " + id + " -x --item"; f.calls[1] != want {
		t.Errorf("pin runs with text that starts with a dash: %q", f.calls[1])
	}
	m, cmd = press(t, settled(t, m), "U")
	m = feed(t, m, cmd)
	if want := "/x/sitrep unpin " + id; f.calls[2] != want {
		t.Errorf("unpin: %q", f.calls[2])
	}
	if i, _ := queueModel(t, qInbox); true {
		for _, k := range []string{"P", "U"} {
			if i, cmd = press(t, i, k); cmd != nil || i.prompt != nil {
				t.Errorf("%s means nothing in the inbox", k)
			}
		}
	}
}

func TestQueuePromptEscEmptyPasteAndCap(t *testing.T) {
	m, f := queueModel(t, qInbox)
	m, _ = press(t, m, "a")
	m = typed(t, m, "abc")
	m, _ = press(t, m, "esc")
	if m.prompt != nil || len(f.calls) != 0 || !m.queue {
		t.Error("esc cancels the prompt and keeps the view")
	}
	m, _ = press(t, m, "a")
	m = typed(t, m, "  ")
	m, cmd := press(t, m, "enter")
	if cmd != nil || m.prompt != nil || !strings.Contains(m.statusText(), "nothing to add") {
		t.Errorf("empty text is refused: %q", m.statusText())
	}
	next, _ := m.Update(tea.PasteMsg{Content: "ignored"})
	if next.(model).prompt != nil {
		t.Error("a paste with no prompt goes nowhere")
	}
	m, _ = press(t, m, "a")
	next, _ = m.Update(tea.PasteMsg{Content: "one\r\ntwo\x1b[31m‮\tthree" + strings.Repeat("y", 300)})
	m = next.(model)
	if !strings.HasPrefix(m.prompt.text, "one two[31m three") || strings.ContainsAny(m.prompt.text, "\n\r\t\x1b‮") {
		t.Errorf("line breaks become spaces and control characters go: %q", m.prompt.text)
	}
	if n := len([]rune(m.prompt.text)); n != maxPrompt {
		t.Errorf("capped at %d: %d", maxPrompt, n)
	}
}

func TestQueueEditKeysAreRefusedWithAReason(t *testing.T) {
	cases := map[string]func(m *model){
		"the dashboard is read-only": func(m *model) { m.readOnly = true },
		"busy":                       func(m *model) { m.acting = true },
		"has ended":                  func(m *model) { m.snap.Sessions[0].Running = false },
		"the session is gone":        func(m *model) { m.snap.Sessions = m.snap.Sessions[1:] },
	}
	for want, set := range cases {
		for _, k := range []string{"a", "E", "x", "K", "J", "T", "P", "U"} {
			m, f := queueModel(t, qSession)
			m, _ = press(t, m, "down")
			set(&m)
			m, cmd := press(t, m, k)
			if m.ask != nil || m.prompt != nil || cmd != nil || len(f.calls) != 0 || !strings.Contains(m.statusText(), want) {
				t.Errorf("%s with %q: ask %v prompt %v status %q", k, want, m.ask, m.prompt, m.statusText())
			}
		}
	}
	// the inbox needs no session, except to take into one
	for want, set := range map[string]func(m *model){
		"there is no session to take it into": func(m *model) { m.qTo = "" },
		"the session is gone":                 func(m *model) { m.qTo = "gone" },
		"has ended":                           func(m *model) { m.snap.Sessions[0].Running = false },
		"the inbox is empty":                  func(m *model) { m.snap.Inbox = Queue{} },
		"the dashboard is read-only":          func(m *model) { m.readOnly = true },
	} {
		m, _ := queueModel(t, qInbox)
		set(&m)
		m, cmd := press(t, m, "t")
		if m.ask != nil || cmd != nil || !strings.Contains(m.statusText(), want) {
			t.Errorf("t with %q: %q", want, m.statusText())
		}
	}
	m, _ := queueModel(t, qInbox)
	m.snap.Sessions[0].Running = false
	if m, _ = press(t, m, "a"); m.prompt == nil {
		t.Error("the inbox can be added to when the target has ended")
	}
}

func TestQueueQuestionIsDroppedWhenTheQueueOrTheTargetChanges(t *testing.T) {
	ask := func(kind int, k string) model {
		m, _ := queueModel(t, kind)
		m, _ = press(t, m, "down")
		m, _ = press(t, m, k)
		if m.ask == nil || m.ask.n != 2 {
			t.Fatalf("the question holds the item and its position: %+v", m.ask)
		}
		return m
	}
	refresh := func(m model, edit func(*Snapshot)) model {
		s := loadFixture(t)
		edit(&s)
		next, _ := m.Update(snapMsg{snap: s})
		return next.(model)
	}
	if m := refresh(ask(qInbox, "t"), func(s *Snapshot) {}); m.ask == nil {
		t.Error("a refresh that changes nothing keeps the question")
	}
	m := refresh(ask(qInbox, "t"), func(s *Snapshot) { s.Inbox.Items = []string{"new first", item2} })
	if m.ask == nil {
		t.Error("the same item at the same position keeps it")
	}
	m = refresh(ask(qInbox, "t"), func(s *Snapshot) { s.Inbox.Items = []string{item2, item1} })
	if m.ask != nil || !strings.Contains(m.statusText(), "the queue changed") {
		t.Errorf("a different item at that position drops it: %q", m.statusText())
	}
	m = refresh(ask(qInbox, "t"), func(s *Snapshot) { s.Sessions[0].Running = false })
	if m.ask != nil || !strings.Contains(m.statusText(), "the session is gone") {
		t.Errorf("an ended target drops it: %q", m.statusText())
	}
	m = refresh(ask(qSession, "x"), func(s *Snapshot) { s.Sessions[0].Queue.Items = []string{sq2, sq1, sq3} })
	if m.ask != nil || !strings.Contains(m.statusText(), "the queue changed") {
		t.Errorf("a remove question follows the session's queue: %q", m.statusText())
	}
	m = refresh(ask(qSession, "x"), func(s *Snapshot) { s.Sessions = s.Sessions[1:] })
	if m.ask != nil {
		t.Error("a session that left drops it")
	}
}

func TestQueueSelectionFollowsItsItemEvenWithDuplicates(t *testing.T) {
	m, _ := queueModel(t, qInbox)
	set := func(items ...string) model {
		s := loadFixture(t)
		s.Inbox.Items, s.Inbox.Count = items, len(items)
		next, _ := m.Update(snapMsg{snap: s})
		return next.(model)
	}
	m = set("a", "b", "c")
	m, _ = press(t, m, "end")
	m = set("z", "a", "b", "c")
	if m.queueIndex() != 3 {
		t.Errorf("an item added above: the selection follows its item: %d", m.queueIndex())
	}
	m = set("b", "b", "c")
	m, _ = press(t, m, "up")
	m = set("a", "b", "b", "b")
	if m.queueIndex() != 1 {
		t.Errorf("a duplicate at the same place keeps the selection there: %d", m.queueIndex())
	}
	m = set("b", "b", "x")
	if m.queueIndex() != 1 {
		t.Errorf("the nearest equal item when its place is gone: %d", m.queueIndex())
	}
	m = set("b", "b")
	if m.queueIndex() != 1 {
		t.Errorf("a duplicate does not pull the selection to the first one: %d", m.queueIndex())
	}
	m = set("q")
	if m.queueIndex() != 0 {
		t.Errorf("and it is clamped when its item is gone: %d", m.queueIndex())
	}
}

func TestQueueKeyInOtherModes(t *testing.T) {
	m := loaded(t, tabCards)
	m, _ = press(t, m, "/")
	m, _ = press(t, m, "e")
	if m.queue || m.filter != "e" {
		t.Errorf("e is text while filtering: queue %v filter %q", m.queue, m.filter)
	}
	m = loaded(t, tabCards)
	m, _ = press(t, m, "?")
	m, _ = press(t, m, "i")
	if m.queue || m.help {
		t.Error("i in the key list only closes it")
	}
	m, _ = press(t, m, "v")
	if m, _ = press(t, m, "e"); !m.queue || !m.detail || m.qKind != qSession {
		t.Error("e opens the session's queue from the detail pane, which stays open under it")
	}
}

func TestQueueScreens(t *testing.T) {
	items := []string{item1, item2, clean("evil \x1b[31mred‮ bidi\ttab"), "日本語の項目: データベースの性能を調べる"}
	v := queueView{Name: "Inbox", Items: items, Total: 4, Sel: 1, Inbox: true, Target: "15f1 claude-sitrep"}
	th := plainTheme()
	golden(t, "queue_inbox_80x24", queueScreen(v, 80, 24, th))
	ask := v
	ask.Ask = "move item 2 to 15f1 claude-sitrep?"
	golden(t, "queue_ask_30x8", queueScreen(ask, 30, 8, th))
	golden(t, "queue_none_80x6", queueScreen(queueView{Name: "Queue 15f1 claude-sitrep"}, 80, 6, th))
	sess := queueView{Name: "Queue 15f1 claude-sitrep", Items: items[:2], Total: 7, Sel: 0}
	golden(t, "queue_session_80x10", queueScreen(sess, 80, 10, th))
	typing := sess
	typing.Prompt, typing.Typed = "add", strings.Repeat("0123456789", 20)
	if out := queueScreen(typing, 20, 8, th); !strings.Contains(out, "9▏") || !strings.Contains(out, "…") {
		t.Errorf("a long line shows its end and the cursor:\n%s", out)
	}
	ro := v
	ro.ReadOnly = true
	if out := queueScreen(ro, 80, 24, th); strings.Contains(out, "take") || strings.Contains(out, "a add") {
		t.Errorf("read-only offers no edit key:\n%s", out)
	}
	msg := v
	msg.Status = "move: already first"
	if out := queueScreen(msg, 80, 24, th); !strings.Contains(out, "move: already first") {
		t.Errorf("a message replaces the key hint:\n%s", out)
	}
	for _, h := range []int{1, 2, 3, 5} {
		for _, w := range []int{1, 2, 10, 20} {
			for _, x := range []queueView{v, ask, typing, sess} {
				out := queueScreen(x, w, h, th)
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
}

func TestHelpHidesEditKeysWhenReadOnly(t *testing.T) {
	if h := helpScreen(100, 40, false, 0, plainTheme()); !strings.Contains(h, "add, edit, remove") || !strings.Contains(h, "take an inbox item") {
		t.Error("the key list names the edit keys")
	}
	if h := helpScreen(100, 40, true, 0, plainTheme()); strings.Contains(h, "add, edit, remove") || strings.Contains(h, "take an inbox item") || !strings.Contains(h, "the inbox, or") {
		t.Error("read-only hides the edit keys but keeps i and e")
	}
}

func TestEKeyEditsTheSelectedItem(t *testing.T) {
	m, f := queueModel(t, qSession)
	id := m.qTo
	m, _ = press(t, m, "down")
	old := m.queueItems()[1]
	m, _ = press(t, m, "E")
	if m.prompt == nil || m.prompt.verb != "edit" || m.prompt.n != 2 || m.prompt.old != old || m.prompt.text != old {
		t.Fatalf("E opens a line with the item's own text: %+v", m.prompt)
	}
	m, _ = press(t, m, "backspace")
	m = typed(t, m, "!")
	if out := m.View().Content; !strings.Contains(out, "edit: ") || !strings.Contains(out, "!▏") {
		t.Errorf("the line is on the footer:\n%s", out)
	}
	m, cmd := press(t, m, "enter")
	m = feed(t, m, cmd)
	want := "/x/sitrep edit " + id + " 2 " + old[:len(old)-1] + "! --item " + old
	if len(f.calls) != 1 || f.calls[0] != want {
		t.Errorf("the one fixed command, guarded by the old text:\n got %v\nwant %s", f.calls, want)
	}
	if m.prompt != nil || m.queueIndex() != 1 {
		t.Errorf("the selection stays on the item: prompt %v, index %d", m.prompt, m.queueIndex())
	}
}

func TestEKeyRefusals(t *testing.T) {
	m, f := queueModel(t, qSession)
	old := m.queueItems()[0]
	m, _ = press(t, m, "E")
	m, cmd := press(t, m, "enter") // unchanged
	if cmd != nil || len(f.calls) != 0 || !strings.Contains(m.statusText(), "unchanged") {
		t.Errorf("unchanged text runs nothing: %v %q", f.calls, m.statusText())
	}
	m, _ = press(t, m, "E")
	for range []rune(old) {
		m, _ = press(t, m, "backspace")
	}
	m, cmd = press(t, m, "enter")
	if cmd != nil || len(f.calls) != 0 || !strings.Contains(m.statusText(), "nothing to edit") {
		t.Errorf("an empty line runs nothing: %v %q", f.calls, m.statusText())
	}
	m, _ = press(t, m, "E")
	m = typed(t, m, "zzz")
	m, cmd = press(t, m, "esc")
	if m.prompt != nil || cmd != nil || len(f.calls) != 0 {
		t.Errorf("esc cancels: %+v %v", m.prompt, f.calls)
	}
	// the inbox can be edited too, and an empty queue has nothing to edit
	mi, fi := queueModel(t, qInbox)
	mi, _ = press(t, mi, "E")
	mi = typed(t, mi, "x")
	mi, cmd = press(t, mi, "enter")
	mi = feed(t, mi, cmd)
	if len(fi.calls) != 1 || !strings.HasPrefix(fi.calls[0], "/x/sitrep edit inbox 1 ") || !strings.Contains(fi.calls[0], " --item ") {
		t.Errorf("inbox: %v", fi.calls)
	}
	me, fe := queueModel(t, qSession)
	me.snap.Sessions[0].Queue = Queue{}
	me, cmd = press(t, me, "E")
	if me.prompt != nil || cmd != nil || len(fe.calls) != 0 || !strings.Contains(me.statusText(), "empty") {
		t.Errorf("an empty queue: %+v %q", me.prompt, me.statusText())
	}
}

func TestQueueStatusSitsBesideTheHints(t *testing.T) {
	v := queueView{Name: "s1", Items: []string{"a"}, Status: "moved to s004"}
	out := queueScreen(v, 100, 6, plainTheme())
	last := out[strings.LastIndex(out, "\n")+1:]
	if !strings.Contains(last, "moved to s004") || !strings.Contains(last, "? help") {
		t.Errorf("the message and the keys share the bar: %q", last)
	}
}

func TestLeavingAQueueDropsItsMessage(t *testing.T) {
	for _, leave := range []string{"q", "esc", "e"} {
		m, _ := queueModel(t, qSession)
		m.say("edited")
		if m.statusText() != "edited" {
			t.Fatal("the message is up")
		}
		m, _ = press(t, m, leave)
		if m.queue || m.statusText() != "" {
			t.Errorf("%s: queue open %v, status %q", leave, m.queue, m.statusText())
		}
	}
}
