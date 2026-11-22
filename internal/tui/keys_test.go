package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestGGAndGOnTheTabs(t *testing.T) {
	m := loaded(t, tabCards)
	ids := m.visible()
	m, _ = press(t, m, "G")
	if m.selID != ids[len(ids)-1] {
		t.Errorf("G selects the last: %q", m.selID)
	}
	m, _ = press(t, m, "g")
	if m.pending == "" || m.selID != ids[len(ids)-1] || m.statusText() != "g-" {
		t.Errorf("a lone g waits: pending %v, %q", m.pending != "", m.statusText())
	}
	if v := m.View().Content; !strings.Contains(v, "g-") {
		t.Errorf("the status bar shows the pending key:\n%s", v)
	}
	m, _ = press(t, m, "g")
	if m.pending != "" || m.selID != ids[0] {
		t.Errorf("gg selects the first: pending %v, %q", m.pending != "", m.selID)
	}
}

func TestAnyOtherKeyAfterGIsSwallowed(t *testing.T) {
	for _, other := range []string{"x", "j", "esc", "q", "d", "n", "?", "enter", "K"} {
		m := loaded(t, tabCards)
		m, _ = press(t, m, "G")
		last := m.selID
		m, _ = press(t, m, "g")
		m, cmd := press(t, m, other)
		if m.pending != "" || cmd != nil || m.selID != last || m.detail || m.queue || m.help || m.prompt != nil || m.filtering {
			t.Errorf("g then %q: pending %v cmd %v sel %q detail %v queue %v help %v", other, m.pending != "", cmd != nil, m.selID, m.detail, m.queue, m.help)
		}
	}
	m := loaded(t, tabCards)
	m, _ = press(t, m, "g")
	if _, cmd := press(t, m, "ctrl+c"); cmd == nil {
		t.Error("ctrl+c still quits with a g waiting")
	}
}

func TestGIsTextWhereTextIsTyped(t *testing.T) {
	m := loaded(t, tabCards)
	m, _ = press(t, m, "/")
	m = typed(t, m, "gg")
	if m.pending != "" || m.filter != "gg" {
		t.Errorf("in the filter g is text: pending %v filter %q", m.pending != "", m.filter)
	}
	m, _ = press(t, m, "esc")
	mq, _ := queueModel(t, qSession)
	mq, _ = press(t, mq, "a")
	mq = typed(t, mq, "gG")
	if mq.pending != "" || mq.prompt == nil || mq.prompt.text != "gG" {
		t.Errorf("in a prompt g is text: %+v", mq.prompt)
	}
	// during a question g does nothing
	m3, _ := queueModel(t, qSession)
	m3, _ = press(t, m3, "x")
	if m3.ask == nil {
		t.Fatal("x asks")
	}
	m3, _ = press(t, m3, "g")
	if m3.pending != "" || m3.ask == nil {
		t.Errorf("g during a question: pending %v ask %v", m3.pending != "", m3.ask)
	}
}

func TestGGAndGInTheQueueAndTheDetailView(t *testing.T) {
	m, _ := queueModel(t, qSession)
	n := len(m.queueItems())
	m, _ = press(t, m, "G")
	if m.queueIndex() != n-1 {
		t.Errorf("G in the queue: %d of %d", m.queueIndex(), n)
	}
	m, _ = press(t, m, "g")
	if v := m.View().Content; !strings.Contains(v, "g-") {
		t.Errorf("the queue footer shows the pending key:\n%s", v)
	}
	m, _ = press(t, m, "g")
	if m.queueIndex() != 0 || m.pending != "" {
		t.Errorf("gg in the queue: %d", m.queueIndex())
	}
	d := loaded(t, tabCards)
	d.height = 10
	d, _ = press(t, d, "v")
	d, _ = press(t, d, "G")
	if d.scroll == 0 {
		t.Fatal("the detail view is not longer than the screen, so there is nothing to scroll")
	}
	d, _ = press(t, d, "g")
	if v := d.View().Content; !strings.Contains(v, "g-") {
		t.Errorf("the detail bar shows the pending key:\n%s", v)
	}
	d, _ = press(t, d, "g")
	if d.scroll != 0 || !d.detail {
		t.Errorf("gg in the detail view scrolls to the top and stays: scroll %d detail %v", d.scroll, d.detail)
	}
}

func TestHAndLSwitchTabs(t *testing.T) {
	m := loaded(t, tabCards)
	m, _ = press(t, m, "l")
	if m.tab != tabTable {
		t.Errorf("l: %d", m.tab)
	}
	m, _ = press(t, m, "h")
	if m.tab != tabCards {
		t.Errorf("h: %d", m.tab)
	}
	m, _ = press(t, m, "h")
	if m.tab != tabWall {
		t.Errorf("h wraps to the last tab: %d", m.tab)
	}
	// they are text in the filter, and do nothing in the queue
	f, _ := press(t, loaded(t, tabCards), "/")
	f = typed(t, f, "hl")
	if f.tab != tabCards || f.filter != "hl" {
		t.Errorf("filter: tab %d %q", f.tab, f.filter)
	}
	q, _ := queueModel(t, qSession)
	q, _ = press(t, q, "l")
	if !q.queue || q.tab != tabCards {
		t.Error("l in the queue does nothing")
	}
}

func TestPagingTheLists(t *testing.T) {
	m := loaded(t, tabTable)
	m.height = 7 // a Table page is height-5 = 2 rows
	ids := m.visible()
	if len(ids) < 3 {
		t.Fatal("the fixture needs three sessions")
	}
	m, _ = press(t, m, "ctrl+d")
	if m.selID != ids[2] {
		t.Errorf("ctrl+d: %q, want %q", m.selID, ids[2])
	}
	m, _ = press(t, m, "ctrl+u")
	if m.selID != ids[0] {
		t.Errorf("ctrl+u: %q", m.selID)
	}
	m, _ = press(t, m, "pgdown")
	if m.selID != ids[2] {
		t.Errorf("pgdown: %q", m.selID)
	}
	m, _ = press(t, m, "pgup")
	m, _ = press(t, m, "pgup")
	if m.selID != ids[0] {
		t.Errorf("pgup stops at the first: %q", m.selID)
	}
	w := loaded(t, tabCards)
	w.height = 7 // a Cards page is one session at this height
	w, _ = press(t, w, "ctrl+d")
	if w.selID != w.visible()[1] {
		t.Errorf("Cards page: %q", w.selID)
	}
	q, _ := queueModel(t, qSession)
	q.height = 4
	q, _ = press(t, q, "ctrl+d")
	if q.queueIndex() != min(1, len(q.queueItems())-1) {
		t.Errorf("queue page: %d", q.queueIndex())
	}
	q, _ = press(t, q, "pgup")
	if q.queueIndex() != 0 {
		t.Errorf("queue page up: %d", q.queueIndex())
	}
}

func TestHelpOpensFromEveryScreenAndReturns(t *testing.T) {
	for name, open := range map[string]func(*testing.T) model{
		"the tabs":  func(t *testing.T) model { return loaded(t, tabCards) },
		"the queue": func(t *testing.T) model { m, _ := queueModel(t, qSession); return m },
		"the inbox": func(t *testing.T) model { m, _ := queueModel(t, qInbox); return m },
		"detail":    func(t *testing.T) model { m, _ := press(t, loaded(t, tabCards), "v"); return m },
	} {
		m := open(t)
		wasQueue, wasDetail := m.queue, m.detail
		m, _ = press(t, m, "?")
		if !m.help || !strings.Contains(m.View().Content, "Keys") {
			t.Errorf("%s: ? opens the help", name)
			continue
		}
		m, cmd := press(t, m, "x") // any key closes it, and does nothing else
		if m.help || cmd != nil || m.queue != wasQueue || m.detail != wasDetail || m.ask != nil {
			t.Errorf("%s: a key closes the help and the screen is as it was: help %v queue %v detail %v ask %v", name, m.help, m.queue, m.detail, m.ask)
		}
		m, _ = press(t, m, "?")
		if _, cmd := press(t, m, "ctrl+c"); cmd == nil {
			t.Errorf("%s: ctrl+c quits from the help", name)
		}
	}
}

func TestOAddsAndEnterEditsInTheQueue(t *testing.T) {
	m, f := queueModel(t, qSession)
	id := m.qTo
	m, _ = press(t, m, "o")
	if m.prompt == nil || m.prompt.verb != "add" {
		t.Fatalf("o opens the add line: %+v", m.prompt)
	}
	m = typed(t, m, "new")
	m, cmd := press(t, m, "enter")
	m = feed(t, m, cmd)
	if len(f.calls) != 1 || f.calls[0] != "/x/sitrep add "+id+" new" {
		t.Errorf("o adds: %v", f.calls)
	}
	m2, _ := queueModel(t, qSession)
	m2, _ = press(t, m2, "a")
	if m2.prompt == nil || m2.prompt.verb != "add" {
		t.Error("a still adds")
	}
	m3, _ := queueModel(t, qSession)
	old := m3.queueItems()[0]
	m3, _ = press(t, m3, "enter")
	if m3.prompt == nil || m3.prompt.verb != "edit" || m3.prompt.text != old {
		t.Errorf("enter edits the selected item: %+v", m3.prompt)
	}
	m4, _ := queueModel(t, qSession)
	m4, _ = press(t, m4, "E")
	if m4.prompt == nil || m4.prompt.verb != "edit" {
		t.Error("E still edits")
	}
	// read-only refuses both
	ro, fr := queueModel(t, qSession)
	ro.readOnly = true
	for _, k := range []string{"o", "enter"} {
		r, cmd := press(t, ro, k)
		if r.prompt != nil || cmd != nil || len(fr.calls) != 0 || !strings.Contains(r.statusText(), "read-only") {
			t.Errorf("%s in read-only: %+v %q", k, r.prompt, r.statusText())
		}
	}
}

func takeModel(t *testing.T, tab int) (model, *fake) {
	t.Helper()
	m := loaded(t, tab)
	m.bin = "/x/sitrep"
	f := &fake{answer: map[string]string{"/x/sitrep move": "moved to 15f1: x\n"}}
	m.run = f.run
	return m, f
}

func TestANumberAsksToTakeAnInboxItem(t *testing.T) {
	for _, tab := range []int{tabCards, tabTable} {
		m, f := takeModel(t, tab)
		id := m.selID
		items := m.snap.Inbox.Items
		if len(items) < 2 {
			t.Fatal("the fixture's inbox needs two items")
		}
		m, cmd := press(t, m, "2")
		if m.ask == nil || m.ask.verb != "take" || m.ask.n != 2 || m.ask.item != items[1] || m.ask.target != id || cmd != nil || m.tab != tab {
			t.Fatalf("tab %d: 2 asks and does not switch tab: %+v tab %d", tab, m.ask, m.tab)
		}
		if v := m.View().Content; !strings.Contains(v, "y/n") || !strings.Contains(v, "move inbox item 2") || !strings.Contains(v, items[1][:10]) {
			t.Errorf("the question is on the status bar:\n%s", v)
		}
		if len(f.calls) != 0 {
			t.Errorf("nothing runs before the answer: %v", f.calls)
		}
		m, cmd = press(t, m, "y")
		m = feed(t, m, cmd)
		want := "/x/sitrep move inbox " + id + " 2 --item " + items[1]
		if len(f.calls) != 1 || f.calls[0] != want {
			t.Errorf("tab %d: the one fixed command, guarded by the item's text:\n got %v\nwant %s", tab, f.calls, want)
		}
		if m.ask != nil || m.statusText() != "moved to 15f1: x" {
			t.Errorf("the answer is shown: ask %v %q", m.ask, m.statusText())
		}
	}
}

func TestTheTakeQuestionAnswers(t *testing.T) {
	for _, k := range []string{"n", "esc", "q"} {
		m, f := takeModel(t, tabCards)
		m, _ = press(t, m, "1")
		m, cmd := press(t, m, k)
		if m.ask != nil || cmd != nil || len(f.calls) != 0 {
			t.Errorf("%s cancels: %v %v", k, m.ask, f.calls)
		}
	}
	m, f := takeModel(t, tabCards)
	m, _ = press(t, m, "1")
	for _, k := range []string{"j", "2", "tab", "d", "e", "a", "g", "x", "enter", "p", "z", "/"} {
		after, cmd := press(t, m, k)
		if after.ask == nil || cmd != nil || after.tab != m.tab || after.detail || after.queue || after.filtering || after.selID != m.selID || len(f.calls) != 0 {
			t.Errorf("%q must do nothing while the question is open", k)
		}
	}
	if _, cmd := press(t, m, "ctrl+c"); cmd == nil {
		t.Error("ctrl+c still quits")
	}
}

func TestANumberRefusals(t *testing.T) {
	cases := map[string]func(m *model){
		"the dashboard is read-only": func(m *model) { m.readOnly = true },
		"busy":                       func(m *model) { m.acting = true },
		"refreshing":                 func(m *model) { m.settling = true },
		"has ended":                  func(m *model) { m.snap.Sessions[2].Running = false },
		"no session is selected":     func(m *model) { m.selID = "" },
	}
	for want, set := range cases {
		m, f := takeModel(t, tabCards)
		set(&m)
		m, cmd := press(t, m, "1")
		if m.ask != nil || cmd != nil || len(f.calls) != 0 || !strings.Contains(m.statusText(), want) {
			t.Errorf("%s: ask %v status %q", want, m.ask, m.statusText())
		}
	}
	m, _ := takeModel(t, tabCards)
	m, _ = press(t, m, "9")
	if m.ask != nil || !strings.Contains(m.statusText(), "no inbox item 9") {
		t.Errorf("a number past the inbox: %v %q", m.ask, m.statusText())
	}
	m2, _ := takeModel(t, tabCards)
	m2.snap.Inbox = Queue{}
	m2, _ = press(t, m2, "1")
	if m2.ask != nil || !strings.Contains(m2.statusText(), "no inbox item 1") {
		t.Errorf("an empty inbox: %v %q", m2.ask, m2.statusText())
	}
}

func TestNumbersAreTextOrNothingElsewhere(t *testing.T) {
	f, _ := press(t, loaded(t, tabCards), "/")
	f = typed(t, f, "15")
	if f.ask != nil || f.filter != "15" {
		t.Errorf("in the filter a number is text: ask %v %q", f.ask, f.filter)
	}
	q, _ := queueModel(t, qSession)
	q, _ = press(t, q, "1")
	if q.ask != nil || !q.queue {
		t.Error("in the queue a number does nothing")
	}
	d, _ := press(t, loaded(t, tabCards), "v")
	d, _ = press(t, d, "1")
	if d.ask != nil || !d.detail {
		t.Error("in the detail view a number does nothing")
	}
}

func TestTheTakeQuestionIsDroppedWhenTheInboxChanges(t *testing.T) {
	m, _ := takeModel(t, tabCards)
	m, _ = press(t, m, "1")
	snap := loadFixture(t)
	snap.Inbox.Items = snap.Inbox.Items[1:]
	next, _ := m.Update(snapMsg{snap: snap})
	m = next.(model)
	if m.ask != nil || !strings.Contains(m.statusText(), "the queue changed") {
		t.Errorf("a changed inbox drops the question: %v %q", m.ask, m.statusText())
	}
}

func moveModel(t *testing.T, kind int) (model, *fake) {
	t.Helper()
	m, f := queueModel(t, kind)
	delete(f.answer, "/x/sitrep") // it answers every command; this one answers only the move
	f.answer["/x/sitrep move"] = "moved to somewhere: x\n"
	return m, f
}

func TestMSendsAnItemToAnotherQueue(t *testing.T) {
	m, f := moveModel(t, qSession)
	from := m.qTo
	m, _ = press(t, m, "down")
	item := m.queueItems()[1]
	m, _ = press(t, m, "m")
	if m.mover == nil || m.mover.from != from || m.mover.n != 2 || m.mover.item != item {
		t.Fatalf("m opens the list: %+v", m.mover)
	}
	// the inbox first, then the other running sessions: never the queue it is in, and never an ended session
	if m.mover.dests[0].id != "inbox" {
		t.Errorf("the inbox comes first: %+v", m.mover.dests)
	}
	for _, d := range m.mover.dests {
		x := m.session(d.id)
		if d.id == from || (d.id != "inbox" && (x == nil || !x.Running)) {
			t.Errorf("a place that is not offered: %+v", d)
		}
	}
	if len(m.mover.dests) < 3 {
		t.Fatalf("the fixture has more running sessions: %+v", m.mover.dests)
	}
	v := m.View().Content
	if !strings.Contains(v, "Send item 2 to") || !strings.Contains(v, "Inbox") || !strings.Contains(v, "enter send here") {
		t.Errorf("the list is drawn:\n%s", v)
	}
	m, _ = press(t, m, "j")
	want := m.mover.dests[1].id
	m, cmd := press(t, m, "enter")
	if cmd == nil || m.mover != nil {
		t.Fatalf("enter sends and closes the list: %v", m.mover)
	}
	m = feed(t, m, cmd)
	if len(f.calls) != 1 || f.calls[0] != "/x/sitrep move "+from+" "+want+" 2 --item "+item {
		t.Errorf("the one fixed command, guarded by the item's text:\n got %v", f.calls)
	}
	if m.statusText() != "moved to somewhere: x" {
		t.Errorf("the answer: %q", m.statusText())
	}
}

func TestMSendsAnItemBackToTheInbox(t *testing.T) {
	m, f := moveModel(t, qSession)
	from, item := m.qTo, m.queueItems()[0]
	m, _ = press(t, m, "m")
	m, cmd := press(t, m, "enter") // the inbox is the first place
	m = feed(t, m, cmd)
	if len(f.calls) != 1 || f.calls[0] != "/x/sitrep move "+from+" inbox 1 --item "+item {
		t.Errorf("back to the inbox: %v", f.calls)
	}
}

func TestMFromTheInboxHasNoInboxPlace(t *testing.T) {
	m, f := moveModel(t, qInbox)
	m, _ = press(t, m, "m")
	if m.mover == nil || m.mover.from != "inbox" {
		t.Fatalf("m in the inbox: %+v", m.mover)
	}
	for _, d := range m.mover.dests {
		if d.id == "inbox" {
			t.Errorf("the inbox offers itself: %+v", m.mover.dests)
		}
	}
	m, _ = press(t, m, "G")
	last := m.mover.dests[len(m.mover.dests)-1].id
	m, cmd := press(t, m, "enter")
	m = feed(t, m, cmd)
	if len(f.calls) != 1 || !strings.HasPrefix(f.calls[0], "/x/sitrep move inbox "+last+" 1 --item ") {
		t.Errorf("%v", f.calls)
	}
}

func TestTheMoveListIsModal(t *testing.T) {
	m, f := moveModel(t, qSession)
	m, _ = press(t, m, "m")
	for _, k := range []string{"x", "a", "o", "E", "K", "t", "T", "P", "i", "e", "d", "z", "1", "tab"} {
		after, cmd := press(t, m, k)
		if after.mover == nil || cmd != nil || after.ask != nil || after.prompt != nil || !after.queue || len(f.calls) != 0 {
			t.Errorf("%q must do nothing while the list is open", k)
		}
	}
	for _, k := range []string{"esc", "q", "backspace"} {
		after, cmd := press(t, m, k)
		if after.mover != nil || cmd != nil || !after.queue || len(f.calls) != 0 {
			t.Errorf("%s cancels and stays in the queue", k)
		}
	}
	if _, cmd := press(t, m, "ctrl+c"); cmd == nil {
		t.Error("ctrl+c quits")
	}
	// the movement keys, bounded, and gg
	m, _ = press(t, m, "G")
	end := m.mover.sel
	m, _ = press(t, m, "j")
	if m.mover.sel != end || end != len(m.mover.dests)-1 {
		t.Errorf("G and j stop at the last: %d", m.mover.sel)
	}
	m, _ = press(t, m, "g")
	m, _ = press(t, m, "g")
	if m.mover.sel != 0 {
		t.Errorf("gg: %d", m.mover.sel)
	}
	m, _ = press(t, m, "k")
	if m.mover.sel != 0 {
		t.Errorf("k stops at the first: %d", m.mover.sel)
	}
	help, _ := press(t, m, "?")
	if !help.help {
		t.Error("? opens the help")
	}
}

func TestMRefusals(t *testing.T) {
	cases := map[string]func(m *model){
		"the dashboard is read-only": func(m *model) { m.readOnly = true },
		"busy":                       func(m *model) { m.acting = true },
		"has ended":                  func(m *model) { m.snap.Sessions[0].Running = false },
		"the queue is empty":         func(m *model) { m.snap.Sessions[0].Queue = Queue{} },
	}
	for want, set := range cases {
		m, f := moveModel(t, qSession)
		set(&m)
		m, cmd := press(t, m, "m")
		if m.mover != nil || cmd != nil || len(f.calls) != 0 || !strings.Contains(m.statusText(), want) {
			t.Errorf("%s: %v %q", want, m.mover, m.statusText())
		}
	}
	// nowhere to send it: only one running session, and the item is in the inbox
	m, _ := moveModel(t, qInbox)
	for i := range m.snap.Sessions {
		m.snap.Sessions[i].Running = false
	}
	m, _ = press(t, m, "m")
	if m.mover != nil || !strings.Contains(m.statusText(), "no other queue") {
		t.Errorf("%v %q", m.mover, m.statusText())
	}
}

func TestTheDestinationMustStillBeRunningWhenTheListIsAnswered(t *testing.T) {
	m, f := moveModel(t, qSession)
	m, _ = press(t, m, "m")
	m, _ = press(t, m, "j")
	gone := m.mover.dests[m.mover.sel].id
	for i := range m.snap.Sessions {
		if m.snap.Sessions[i].ID == gone {
			m.snap.Sessions[i].Running = false
		}
	}
	m, cmd := press(t, m, "enter")
	if cmd != nil || len(f.calls) != 0 || !strings.Contains(m.statusText(), "has ended") {
		t.Errorf("%v %q", f.calls, m.statusText())
	}
}

func TestTheMoveListIsDroppedWhenTheQueueChanges(t *testing.T) {
	m, _ := moveModel(t, qSession)
	m, _ = press(t, m, "m")
	snap := loadFixture(t)
	snap.Sessions[0].Queue.Items = snap.Sessions[0].Queue.Items[1:]
	next, _ := m.Update(snapMsg{snap: snap})
	m = next.(model)
	if m.mover != nil || !strings.Contains(m.statusText(), "the queue changed") {
		t.Errorf("%v %q", m.mover, m.statusText())
	}
}

func TestTheMoveListFitsTheScreen(t *testing.T) {
	m, _ := moveModel(t, qSession)
	m, _ = press(t, m, "m")
	for _, w := range []int{1, 2, 10, 20, 30, 60, 80, 120} {
		for _, h := range []int{1, 2, 3, 6, 10, 24} {
			mm := m
			mm.width, mm.height = w, h
			lines := strings.Split(mm.View().Content, "\n")
			if len(lines) > h {
				t.Errorf("%dx%d: %d lines", w, h, len(lines))
			}
			for _, l := range lines {
				if ansi.StringWidth(l) > w {
					t.Errorf("%dx%d: a line is %d wide: %q", w, h, ansi.StringWidth(l), l)
				}
			}
		}
	}
}

func TestDDRemovesAnItemAndAsks(t *testing.T) {
	m, f := queueModel(t, qSession)
	id := m.qTo
	m, _ = press(t, m, "down")
	item := m.queueItems()[1]
	m, _ = press(t, m, "d")
	if m.pending != "d" || m.statusText() != "d-" || m.ask != nil {
		t.Fatalf("a lone d waits: %q %v", m.pending, m.ask)
	}
	if v := m.View().Content; !strings.Contains(v, "d-") {
		t.Errorf("the queue footer shows it:\n%s", v)
	}
	m, _ = press(t, m, "d")
	if m.pending != "" || m.ask == nil || m.ask.verb != "remove" || m.ask.n != 2 || m.ask.item != item || len(f.calls) != 0 {
		t.Fatalf("dd asks, and runs nothing yet: %+v %v", m.ask, f.calls)
	}
	m, cmd := press(t, m, "y")
	m = feed(t, m, cmd)
	if want := "/x/sitrep rm " + id + " 2 --item " + item; len(f.calls) != 1 || f.calls[0] != want {
		t.Errorf("the one fixed command, guarded by the item's text:\n got %v\nwant %s", f.calls, want)
	}
}

func TestDThenAnotherKeyIsSwallowed(t *testing.T) {
	for _, other := range []string{"x", "y", "j", "esc", "o", "E", "m", "q", "enter", "g"} {
		m, f := queueModel(t, qSession)
		m, _ = press(t, m, "d")
		m, cmd := press(t, m, other)
		if m.pending != "" || cmd != nil || m.ask != nil || m.prompt != nil || m.mover != nil || !m.queue || len(f.calls) != 0 {
			t.Errorf("d then %q: pending %q ask %v prompt %v mover %v queue %v calls %v", other, m.pending, m.ask, m.prompt, m.mover, m.queue, f.calls)
		}
	}
	m, _ := queueModel(t, qSession)
	m, _ = press(t, m, "d")
	if _, cmd := press(t, m, "ctrl+c"); cmd == nil {
		t.Error("ctrl+c still quits with a d waiting")
	}
}

func TestDIsNotPendingWhereItDoesNothing(t *testing.T) {
	// on the tabs and in the detail view d is nothing now, and the detail view is v
	m := loaded(t, tabCards)
	m, cmd := press(t, m, "d")
	if m.pending != "" || m.detail || cmd != nil {
		t.Errorf("d on a tab: %q detail %v", m.pending, m.detail)
	}
	m, _ = press(t, m, "v")
	if !m.detail {
		t.Fatal("v opens the detail view")
	}
	m, _ = press(t, m, "d")
	if !m.detail || m.pending != "" {
		t.Errorf("d in the detail view does nothing: detail %v pending %q", m.detail, m.pending)
	}
	m, _ = press(t, m, "v")
	if m.detail {
		t.Error("v closes it again")
	}
	// the question and the list of places are modal
	q, _ := queueModel(t, qSession)
	q, _ = press(t, q, "x")
	q, _ = press(t, q, "d")
	if q.pending != "" || q.ask == nil {
		t.Errorf("d during a question: %q %v", q.pending, q.ask)
	}
	mv, _ := moveModel(t, qSession)
	mv, _ = press(t, mv, "m")
	mv, _ = press(t, mv, "d")
	if mv.pending != "" || mv.mover == nil {
		t.Errorf("d in the list of places: %q", mv.pending)
	}
	// in a text line d is text
	p, _ := queueModel(t, qSession)
	p, _ = press(t, p, "a")
	p = typed(t, p, "dd")
	if p.pending != "" || p.prompt == nil || p.prompt.text != "dd" {
		t.Errorf("in a prompt d is text: %+v", p.prompt)
	}
}

func TestXAndDeleteStillRemove(t *testing.T) {
	for _, k := range []string{"x", "delete"} {
		m, f := queueModel(t, qSession)
		m, _ = press(t, m, k)
		if m.ask == nil || m.ask.verb != "remove" || len(f.calls) != 0 {
			t.Errorf("%s asks to remove: %+v", k, m.ask)
		}
	}
	// read-only refuses all of them
	for _, keys := range [][]string{{"x"}, {"delete"}, {"d", "d"}} {
		m, f := queueModel(t, qSession)
		m.readOnly = true
		for _, k := range keys {
			m, _ = press(t, m, k)
		}
		if m.ask != nil || len(f.calls) != 0 || !strings.Contains(m.statusText(), "read-only") {
			t.Errorf("%v in read-only: %v %q", keys, m.ask, m.statusText())
		}
	}
}

func TestEAndIOpenQueuesFromTheDetailView(t *testing.T) {
	m, f := moveModel(t, qSession)
	m, _ = press(t, m, "esc") // the queue screen first, only to get a model with the fake runner
	m, _ = press(t, m, "v")
	if !m.detail || m.queue {
		t.Fatal("v opens the detail view")
	}
	id := m.selID
	m, _ = press(t, m, "e")
	if !m.queue || m.qKind != qSession || m.qTo != id || !m.detail {
		t.Fatalf("e opens the session's queue, with the detail view under it: queue %v kind %d to %q detail %v", m.queue, m.qKind, m.qTo, m.detail)
	}
	if v := m.View().Content; !strings.Contains(v, "Queue ") {
		t.Errorf("the queue is drawn:\n%s", v)
	}
	// send the first item back to the inbox: m, then enter on the first place
	item := m.queueItems()[0]
	m, _ = press(t, m, "m")
	m, cmd := press(t, m, "enter")
	m = feed(t, m, cmd)
	if len(f.calls) != 1 || f.calls[0] != "/x/sitrep move "+id+" inbox 1 --item "+item {
		t.Errorf("back to the inbox from the detail view: %v", f.calls)
	}
	// q closes the queue and comes back to the detail view, and q again to the tabs
	m, _ = press(t, m, "q")
	if m.queue || !m.detail {
		t.Errorf("q from the queue returns to the detail view: queue %v detail %v", m.queue, m.detail)
	}
	m, _ = press(t, m, "q")
	if m.detail || m.queue {
		t.Error("q again closes the detail view")
	}
	// the inbox too
	d, _ := press(t, loaded(t, tabCards), "v")
	d, _ = press(t, d, "i")
	if !d.queue || d.qKind != qInbox || !d.detail {
		t.Errorf("i opens the inbox from the detail view: %v %d", d.queue, d.qKind)
	}
	// the detail footer says so
	e, _ := press(t, loaded(t, tabCards), "v")
	e.width, e.height = 100, 30
	if v := e.View().Content; !strings.Contains(v, "e queue") {
		t.Errorf("the detail bar names e:\n%s", v)
	}
}
