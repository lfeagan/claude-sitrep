package tui

import (
	"strings"
	"testing"
)

func TestGGAndGOnTheTabs(t *testing.T) {
	m := loaded(t, tabWatch)
	ids := m.visible()
	m, _ = press(t, m, "G")
	if m.selID != ids[len(ids)-1] {
		t.Errorf("G selects the last: %q", m.selID)
	}
	m, _ = press(t, m, "g")
	if !m.pendingG || m.selID != ids[len(ids)-1] || m.statusText() != "g-" {
		t.Errorf("a lone g waits: pending %v, %q", m.pendingG, m.statusText())
	}
	if v := m.View().Content; !strings.Contains(v, "g-") {
		t.Errorf("the status bar shows the pending key:\n%s", v)
	}
	m, _ = press(t, m, "g")
	if m.pendingG || m.selID != ids[0] {
		t.Errorf("gg selects the first: pending %v, %q", m.pendingG, m.selID)
	}
}

func TestAnyOtherKeyAfterGIsSwallowed(t *testing.T) {
	for _, other := range []string{"x", "j", "esc", "q", "d", "n", "?", "enter", "K"} {
		m := loaded(t, tabWatch)
		m, _ = press(t, m, "G")
		last := m.selID
		m, _ = press(t, m, "g")
		m, cmd := press(t, m, other)
		if m.pendingG || cmd != nil || m.selID != last || m.detail || m.queue || m.help || m.prompt != nil || m.filtering {
			t.Errorf("g then %q: pending %v cmd %v sel %q detail %v queue %v help %v", other, m.pendingG, cmd != nil, m.selID, m.detail, m.queue, m.help)
		}
	}
	m := loaded(t, tabWatch)
	m, _ = press(t, m, "g")
	if _, cmd := press(t, m, "ctrl+c"); cmd == nil {
		t.Error("ctrl+c still quits with a g waiting")
	}
}

func TestGIsTextWhereTextIsTyped(t *testing.T) {
	m := loaded(t, tabWatch)
	m, _ = press(t, m, "/")
	m = typed(t, m, "gg")
	if m.pendingG || m.filter != "gg" {
		t.Errorf("in the filter g is text: pending %v filter %q", m.pendingG, m.filter)
	}
	m, _ = press(t, m, "esc")
	mq, _ := queueModel(t, qSession)
	mq, _ = press(t, mq, "a")
	mq = typed(t, mq, "gG")
	if mq.pendingG || mq.prompt == nil || mq.prompt.text != "gG" {
		t.Errorf("in a prompt g is text: %+v", mq.prompt)
	}
	// during a question g does nothing
	m3, _ := queueModel(t, qSession)
	m3, _ = press(t, m3, "x")
	if m3.ask == nil {
		t.Fatal("x asks")
	}
	m3, _ = press(t, m3, "g")
	if m3.pendingG || m3.ask == nil {
		t.Errorf("g during a question: pending %v ask %v", m3.pendingG, m3.ask)
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
	if m.queueIndex() != 0 || m.pendingG {
		t.Errorf("gg in the queue: %d", m.queueIndex())
	}
	d := loaded(t, tabWatch)
	d.height = 10
	d, _ = press(t, d, "d")
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
	m := loaded(t, tabWatch)
	m, _ = press(t, m, "l")
	if m.tab != tabTop {
		t.Errorf("l: %d", m.tab)
	}
	m, _ = press(t, m, "h")
	if m.tab != tabWatch {
		t.Errorf("h: %d", m.tab)
	}
	m, _ = press(t, m, "h")
	if m.tab != tabTop {
		t.Errorf("h wraps: %d", m.tab)
	}
	// they are text in the filter, and do nothing in the queue
	f, _ := press(t, loaded(t, tabWatch), "/")
	f = typed(t, f, "hl")
	if f.tab != tabWatch || f.filter != "hl" {
		t.Errorf("filter: tab %d %q", f.tab, f.filter)
	}
	q, _ := queueModel(t, qSession)
	q, _ = press(t, q, "l")
	if !q.queue || q.tab != tabWatch {
		t.Error("l in the queue does nothing")
	}
}

func TestPagingTheLists(t *testing.T) {
	m := loaded(t, tabTop)
	m.height = 7 // a Top page is height-5 = 2 rows
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
	w := loaded(t, tabWatch)
	w.height = 7 // a Watch page is one session at this height
	w, _ = press(t, w, "ctrl+d")
	if w.selID != w.visible()[1] {
		t.Errorf("Watch page: %q", w.selID)
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
		"the tabs":  func(t *testing.T) model { return loaded(t, tabWatch) },
		"the queue": func(t *testing.T) model { m, _ := queueModel(t, qSession); return m },
		"the inbox": func(t *testing.T) model { m, _ := queueModel(t, qInbox); return m },
		"detail":    func(t *testing.T) model { m, _ := press(t, loaded(t, tabWatch), "d"); return m },
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
