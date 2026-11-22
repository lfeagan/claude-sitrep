package tui

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/lfeagan/claude-sitrep/internal/conf"
	"github.com/lfeagan/claude-sitrep/internal/settings"
	"github.com/lfeagan/claude-sitrep/internal/snapshot"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// memFile is a ColorsFile in memory that counts what is done to it.
type memFile struct {
	body       string
	backups    []string
	writes     int
	writeErr   error
	beforeRead func() // runs at the next Read, to play the user's editor
	otherEdit  bool   // a write also changes another line
}

func (f *memFile) Path() string { return "/home/you/sitrep.conf" }
func (f *memFile) Values() (string, string) {
	value := func(name string) string {
		return strings.Trim(strings.TrimPrefix(conf.Line(f.body, name), name+"="), "'")
	}
	return value("SITREP_THEME"), value("SITREP_COLORS")
}
func (f *memFile) Read() (string, error) {
	if f.beforeRead != nil {
		f.beforeRead()
		f.beforeRead = nil
	}
	return f.body, nil
}
func (f *memFile) Write(out, original string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	if original != f.body {
		return settings.ErrChanged
	}
	f.writes++
	f.body = out
	if f.otherEdit {
		f.body += "SNEAKY=1\n"
	}
	return nil
}
func (f *memFile) Backup(body string) (string, error) {
	f.backups = append(f.backups, body)
	return "/home/you/sitrep.conf.bak.1", nil
}

func demoSessions() snapshot.Snapshot {
	mk := func(id, state string, needs bool) snapshot.Session {
		return snapshot.Session{ID: demoIDForTest(id), Short: id[:4], Running: true, State: state, StateLabel: state, NeedsInput: needs,
			Dir: textsafe.Sane("web", 20), Objective: textsafe.Sane("o", 10), Now: textsafe.Sane("n", 10)}
	}
	return snapshot.Snapshot{Version: 1, Registry: "ok", Sessions: []snapshot.Session{mk("aaaa1111", "waiting", true), mk("bbbb2222", "working", false)}}
}

func newColors(t *testing.T, body string) (colorsModel, *memFile) {
	t.Helper()
	f := &memFile{body: body}
	m := newColorsModel(ColorsOptions{Sessions: demoSessions(), File: f})
	m.width, m.height = 100, 30
	return m, f
}

func ckey(t *testing.T, m colorsModel, keys ...string) colorsModel {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "up":
			msg = tea.KeyPressMsg{Code: tea.KeyUp}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "ctrl+u":
			msg = tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
		default:
			msg = tea.KeyPressMsg{Code: rune(k[0]), Text: k}
		}
		next, _ := m.Update(msg)
		m = next.(colorsModel)
	}
	return m
}

// typeColor edits the selected colour to text.
func typeColor(t *testing.T, m colorsModel, text string) colorsModel {
	t.Helper()
	m = ckey(t, m, "enter", "ctrl+u")
	for _, r := range text {
		m = ckey(t, m, string(r))
	}
	return ckey(t, m, "enter")
}

func TestColorsStartsFromTheFile(t *testing.T) {
	m, _ := newColors(t, "A=1\nSITREP_THEME=rainbow\nSITREP_COLORS='attn=#112233/#445566'\n")
	if m.theme != "rainbow" || m.colorsString() != "attn=#112233/#445566" {
		t.Errorf("theme %q colours %q", m.theme, m.colorsString())
	}
	if got := m.value("attn")[1]; got != "#445566" {
		t.Errorf("a set colour: %s", got)
	}
	if got := m.value("dim"); got != presets["rainbow"].dim {
		t.Errorf("a colour that is not set is the theme's: %v", got)
	}
	if m.dirty() {
		t.Error("nothing is changed yet")
	}
	if m2, _ := newColors(t, "SITREP_THEME=nonsense\n"); m2.theme != "default" {
		t.Errorf("an unknown theme is the default: %q", m2.theme)
	}
}

func TestColorsEditing(t *testing.T) {
	m, _ := newColors(t, "")
	m = ckey(t, m, "j", "j", "j", "j") // attn
	m = typeColor(t, m, "#FF0000")
	if got := m.value("attn"); got[1] != "#ff0000" || got[0] != presets["default"].attn[0] {
		t.Errorf("one side is edited, the other is kept: %v", got)
	}
	m = ckey(t, m, "t") // the light side
	m = typeColor(t, m, "#ff0000")
	if got := m.colorsString(); got != "attn=#ff0000" {
		t.Errorf("equal sides are one value: %q", got)
	}
	m = typeColor(t, m, "red")
	if m.edit == nil || !strings.Contains(m.msg, "not a colour") {
		t.Errorf("junk is refused and the field stays: %q", m.msg)
	}
	m = ckey(t, m, "esc", "r")
	if _, ok := m.set["attn"]; ok || m.colorsString() != "" {
		t.Errorf("r clears the setting: %q", m.colorsString())
	}
	// a number walks with up and down
	m = ckey(t, m, "enter", "ctrl+u", "2", "0", "up", "up", "down")
	if m.edit.text != "19" {
		t.Errorf("up and down walk the numbers: %q", m.edit.text)
	}
}

func TestColorsThemeRow(t *testing.T) {
	m, _ := newColors(t, "SITREP_COLORS=dim=7\n")
	m = ckey(t, m, "l")
	if m.theme != "pastel" {
		t.Errorf("l is the next theme: %q", m.theme)
	}
	if m.colorsString() != "dim=7" {
		t.Error("a theme change keeps the colours the user set")
	}
	m = ckey(t, m, "h", "h")
	if m.theme != "mono" {
		t.Errorf("themes wrap: %q", m.theme)
	}
	m = ckey(t, m, "j", "enter")
	if m.edit != nil || !strings.Contains(m.msg, "mono has no colours") {
		t.Errorf("mono cannot be edited: %q", m.msg)
	}
	if !strings.Contains(ansi.Strip(m.body()), "mono has no colours") {
		t.Error("the rows say so")
	}
	m = ckey(t, m, "g", "r")
	if m.theme != "default" {
		t.Errorf("r on the theme row: %q", m.theme)
	}
}

func TestColorsWriteAsksBacksUpAndWrites(t *testing.T) {
	m, f := newColors(t, "MINE=1\n")
	m = ckey(t, m, "w")
	if m.ask != nil || !strings.Contains(m.msg, "nothing to write") {
		t.Fatalf("nothing changed, nothing asked: %q", m.msg)
	}
	m = ckey(t, m, "l", "w") // the pastel theme
	if m.ask == nil || m.ask.kind != "write" {
		t.Fatal("w asks")
	}
	q := ansi.Strip(m.body())
	if !strings.Contains(q, "was  (none)") || !strings.Contains(q, "now  SITREP_THEME=pastel") || !strings.Contains(q, "/home/you/sitrep.conf") {
		t.Errorf("the question names the file and the lines:\n%s", q)
	}
	if f.writes != 0 || len(f.backups) != 0 {
		t.Error("nothing happens before the y")
	}
	m = ckey(t, m, "n")
	if m.ask != nil || f.writes != 0 {
		t.Error("n cancels")
	}
	m = ckey(t, m, "w", "y")
	if f.writes != 1 || f.body != "MINE=1\nSITREP_THEME=pastel\n" || len(f.backups) != 1 || f.backups[0] != "MINE=1\n" {
		t.Errorf("writes %d body %q backups %q", f.writes, f.body, f.backups)
	}
	if !strings.Contains(m.msg, "written") || !strings.Contains(m.msg, "backup") || m.dirty() {
		t.Errorf("%q dirty %v", m.msg, m.dirty())
	}
	m = ckey(t, m, "l", "w", "y")
	if f.writes != 2 || len(f.backups) != 2 || f.backups[1] != "MINE=1\nSITREP_THEME=pastel\n" {
		t.Errorf("each write saves the bytes it replaces: writes %d backups %q", f.writes, f.backups)
	}
}

func TestColorsWriteAsksAgainWhenTheFileChangedUnderTheQuestion(t *testing.T) {
	m, f := newColors(t, "MINE=1\n")
	m = ckey(t, m, "l", "w")
	f.body = "MINE=1\nEDITED=1\n" // the user's editor, while the question is open
	m = ckey(t, m, "y")
	if f.writes != 0 || m.ask == nil || !strings.Contains(m.msg, "changed while you looked") {
		t.Fatalf("writes %d ask %v msg %q", f.writes, m.ask, m.msg)
	}
	m = ckey(t, m, "y")
	if f.writes != 1 || !strings.Contains(f.body, "EDITED=1") || !strings.Contains(f.body, "SITREP_THEME=pastel") {
		t.Errorf("the second yes writes over what the file holds now: %q", f.body)
	}
}

func TestColorsWriteStopsWhenTheFileChangesAtTheLastMoment(t *testing.T) {
	m, f := newColors(t, "MINE=1\n")
	m = ckey(t, m, "l", "w")
	f.writeErr = settings.ErrChanged
	m = ckey(t, m, "y")
	if !strings.Contains(m.msg, "nothing was written") || f.body != "MINE=1\n" || !m.dirty() {
		t.Errorf("%q %q dirty %v", m.msg, f.body, m.dirty())
	}
	f.writeErr = errors.New("read-only folder")
	m = ckey(t, m, "w", "y")
	if !strings.Contains(m.msg, "nothing written: read-only folder") {
		t.Errorf("%q", m.msg)
	}
}

func TestColorsWriteReportsAChangedOtherLine(t *testing.T) {
	m, f := newColors(t, "MINE=1\n")
	f.otherEdit = true
	m = ckey(t, m, "l", "w", "y")
	if !strings.Contains(m.msg, "not as it should be") || !strings.Contains(m.msg, "bak") {
		t.Errorf("%q", m.msg)
	}
}

func TestColorsReadOnlyAndQuit(t *testing.T) {
	f := &memFile{}
	m := newColorsModel(ColorsOptions{Sessions: demoSessions(), File: f, ReadOnly: true})
	m = ckey(t, m, "l", "w")
	if m.ask != nil || !strings.Contains(m.msg, "read-only") {
		t.Errorf("w is off: %q", m.msg)
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); cmd != nil {
		t.Error("q with a change asks first")
	}
	m = ckey(t, m, "q")
	if m.ask == nil || m.ask.kind != "quit" {
		t.Fatal("q asks when something is unwritten")
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"}); cmd == nil {
		t.Error("y quits")
	}
	clean, _ := newColors(t, "")
	if _, cmd := clean.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); cmd == nil {
		t.Error("q quits when nothing is changed")
	}
}

func TestColorsScreen(t *testing.T) {
	m, _ := newColors(t, "SITREP_COLORS='snoozed=#7aa2f7'\n")
	golden(t, "colors_100x30", ansi.Strip(m.body()))
	m.width, m.height = 60, 20
	golden(t, "colors_60x20", ansi.Strip(m.body()))
	m.height = 12
	if got := m.body(); !strings.Contains(got, "taller") {
		t.Errorf("a short screen says so: %q", got)
	}
	// every line fits the width, at any size that is shown
	for _, w := range []int{20, 40, 80, 150} {
		for _, h := range []int{18, 24, 50} {
			m.width, m.height = w, h
			for _, l := range strings.Split(m.body(), "\n") {
				if ansi.StringWidth(l) > w {
					t.Fatalf("%dx%d: %q is wider than the screen", w, h, l)
				}
			}
			if n := len(strings.Split(m.body(), "\n")); n != h {
				t.Fatalf("%dx%d: %d lines", w, h, n)
			}
		}
	}
}

func TestColorsPreviewFollowsTheSetting(t *testing.T) {
	m, _ := newColors(t, "")
	plain := m.body()
	m = ckey(t, m, "j", "j", "j", "j")
	m = typeColor(t, m, "#00ff00")
	if m.body() == plain {
		t.Error("the preview changes with the colour")
	}
	if !strings.Contains(m.body(), "48;2;0;255;0") {
		t.Error("the needs-you rows are painted with the chosen background")
	}
}

func TestColorsFileOnDisk(t *testing.T) {
	// the real file type that the command wraps keeps a link a link
	dir := t.TempDir()
	c := conf.Open(dir)
	target := filepath.Join(t.TempDir(), "dot")
	os.WriteFile(target, []byte("A=1\n"), 0o600)
	os.Symlink(target, c.File)
	out, _, _ := conf.Edit("A=1\n", []conf.Change{{Name: "SITREP_THEME", Value: ptr("mono")}})
	if err := c.WriteBody(out, "A=1\n"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); string(got) != "A=1\nSITREP_THEME=mono\n" {
		t.Errorf("%q", got)
	}
}

func ptr(s string) *string { return &s }

func demoIDForTest(s string) state.ID { id, _ := state.ParseID(s); return id }

func TestColorsNeverDrawsTextFromTheFileRaw(t *testing.T) {
	m, _ := newColors(t, "SITREP_COLORS=attn=#112233\x1b]52;c;x\x07\x1b[31m\n")
	m = ckey(t, m, "j", "j", "j", "j")
	m = typeColor(t, m, "#445566")
	m = ckey(t, m, "w")
	if m.ask == nil {
		t.Fatalf("no question: %q", m.msg)
	}
	if !strings.Contains(ansi.Strip(m.body()), "was  SITREP_COLORS=attn=#112233") {
		t.Errorf("the old line is shown, cleaned: %q", ansi.Strip(m.body()))
	}
	if strings.Contains(m.body(), "\x1b]") || strings.Contains(m.body(), "\x07") || strings.Contains(m.body(), "52;c") && strings.Contains(m.body(), "\x1b[31m") {
		t.Errorf("the question draws the file's escape bytes: %q", m.body())
	}
	long := &memFile{body: "SITREP_COLORS=" + strings.Repeat("a", 5000) + "\n"}
	m2 := newColorsModel(ColorsOptions{Sessions: demoSessions(), File: long})
	m2.width, m2.height = 100, 30
	m2 = ckey(t, m2, "l", "w")
	for _, l := range strings.Split(m2.body(), "\n") {
		if ansi.StringWidth(l) > 100 {
			t.Fatalf("a long line overflows: %d", ansi.StringWidth(l))
		}
	}
}

func TestColorsOnlyWriteLeavesTheThemeLineAlone(t *testing.T) {
	m, f := newColors(t, "SITREP_THEME=Mono-from-the-future\nSITREP_THEME=again\nA=1\n")
	m = ckey(t, m, "j", "j", "j", "j")
	m = typeColor(t, m, "#445566")
	m = ckey(t, m, "w", "y")
	if !strings.HasPrefix(f.body, "SITREP_THEME=Mono-from-the-future\nSITREP_THEME=again\nA=1\n") || !strings.Contains(f.body, "SITREP_COLORS=") {
		t.Errorf("only the colours are changed: %q", f.body)
	}
}

func TestColorsReadBackIsQuietWhenTheFileHadNoFinalNewline(t *testing.T) {
	m, f := newColors(t, "A=1")
	m = ckey(t, m, "l", "w", "y")
	if !strings.HasPrefix(m.msg, "written (") || f.body != "A=1\nSITREP_THEME=pastel\n" {
		t.Errorf("%q %q", m.msg, f.body)
	}
}

func TestColorsBackupFollowsEveryWriteAndEveryRetry(t *testing.T) {
	m, f := newColors(t, "A=1\n")
	m = ckey(t, m, "l", "w")
	f.writeErr = settings.ErrChanged
	m = ckey(t, m, "y")
	f.writeErr = nil
	f.body = "A=1\nEDITED=1\n"
	m = ckey(t, m, "w", "y")
	if len(f.backups) != 2 || f.backups[1] != "A=1\nEDITED=1\n" {
		t.Errorf("the retry saves what the file holds now: %q", f.backups)
	}
}

func TestColorsEditsDoNotChangeAnEarlierModel(t *testing.T) {
	m, _ := newColors(t, "")
	m = ckey(t, m, "j", "j", "j", "j")
	before := m
	m = typeColor(t, m, "#445566")
	if _, ok := before.set["attn"]; ok {
		t.Error("the model before the edit changed too")
	}
}

func TestColorsFieldStepsThroughHueLightnessAndChroma(t *testing.T) {
	m, _ := newColors(t, "")
	m = ckey(t, m, "j", "j", "j", "j") // attn
	m = ckey(t, m, "enter", "ctrl+u", "#", "5", "b", "3", "f", "d", "1")
	start := m.edit.text
	for k, name := range map[string]string{"l": "hue", "h": "hue", "k": "light", "j": "light", "L": "chroma", "H": "chroma", "right": "hue", "up": "light"} {
		after := ckey(t, m, k)
		if after.edit == nil || after.edit.text == start || normColor(after.edit.text) == "" {
			t.Errorf("%s (%s) did not move the colour: %q", k, name, after.edit.text)
		}
	}
	if len(m.edit.text) != 7 {
		t.Fatal("the field is a hex colour")
	}
	// the preview shows the candidate before it is accepted
	before := m.body()
	m = ckey(t, m, "l", "l", "l")
	if m.body() == before {
		t.Error("the preview follows the field")
	}
	if _, ok := m.set["attn"]; ok {
		t.Error("nothing is set until enter")
	}
	m = ckey(t, m, "esc")
	if m.body() != newColorsPreview(t) {
		t.Error("esc puts the preview back")
	}
	// a number is walked with up and down; l turns the hue of that colour as hex
	m = ckey(t, m, "enter", "ctrl+u", "1", "9", "6")
	m = ckey(t, m, "down")
	if m.edit.text != "197" {
		t.Errorf("numbers walk: %q", m.edit.text)
	}
	m = ckey(t, m, "l")
	if normColor(m.edit.text) == "" || m.edit.text[0] != '#' {
		t.Errorf("a hue step makes it hex: %q", m.edit.text)
	}
}

func newColorsPreview(t *testing.T) string {
	m, _ := newColors(t, "")
	m = ckey(t, m, "j", "j", "j", "j")
	return m.body()
}

func TestColorsFieldTurnsRoundAndBackToTheSameColour(t *testing.T) {
	m, _ := newColors(t, "")
	m = ckey(t, m, "j", "j", "j", "j")
	m = ckey(t, m, "enter", "ctrl+u", "#", "7", "a", "6", "f", "d", "8")
	for i := 0; i < 36; i++ {
		m = ckey(t, m, "l")
	}
	if a, d := channels("#7a6fd8"), channels(m.edit.text); a != d && (a[0]-d[0] > 1 || d[0]-a[0] > 1 || a[1]-d[1] > 1 || d[1]-a[1] > 1 || a[2]-d[2] > 1 || d[2]-a[2] > 1) {
		t.Errorf("a full turn of the hue gave %s", m.edit.text)
	}
	// typing a new colour starts from what was typed, not from the earlier steps
	m = ckey(t, m, "ctrl+u", "#", "0", "0", "f", "f", "0", "0")
	m = ckey(t, m, "k")
	if k := hexToOKLCH(m.edit.text); math.Abs(k.h-hexToOKLCH("#00ff00").h) > 3 {
		t.Errorf("a typed colour is the new start: %s", m.edit.text)
	}
}

func colorsDash(t *testing.T, readOnly bool) (model, *memFile) {
	t.Helper()
	f := &memFile{body: "A=1\n"}
	m := loaded(t, tabTable)
	m.colorsOpts = &ColorsOptions{File: f}
	m.readOnly = readOnly
	m.run = func(context.Context, string, ...string) (string, error) { return "", nil }
	return m, f
}

func TestCKeyOpensTheColourToolOverTheDashboardAndReturns(t *testing.T) {
	m, f := colorsDash(t, false)
	m, _ = press(t, m, "C")
	if m.colors == nil {
		t.Fatal("C opens the tool")
	}
	if m.colors.snap.Sessions[0].ID != m.snap.Sessions[0].ID {
		t.Error("the preview is made of the dashboard's own sessions")
	}
	if !strings.Contains(m.View().Content, "Colours") {
		t.Error("the tool is on screen")
	}
	sel := m.selID
	m, _ = press(t, m, "j") // goes to the tool, not to the dashboard
	if m.selID != sel || m.colors.sel != 1 {
		t.Errorf("keys go to the tool: dashboard %q tool row %d", m.selID, m.colors.sel)
	}
	m, _ = press(t, m, "g") // the theme row
	m, _ = press(t, m, "l")
	m, _ = press(t, m, "w")
	m, _ = press(t, m, "y")
	if f.body != "A=1\nSITREP_THEME=pastel\n" {
		t.Fatalf("the write works from the dashboard: %q", f.body)
	}
	m, cmd := press(t, m, "q")
	if m.colors != nil || cmd == nil {
		t.Errorf("q returns to the dashboard and fetches again: tool %v cmd %v", m.colors != nil, cmd != nil)
	}
	if m.tab != tabTable {
		t.Error("the dashboard is where it was")
	}
}

func TestCKeyInAReadOnlyDashboardOpensAReadOnlyTool(t *testing.T) {
	m, f := colorsDash(t, true)
	m, _ = press(t, m, "C")
	m, _ = press(t, m, "l")
	m, _ = press(t, m, "w")
	if f.writes != 0 || !strings.Contains(m.colors.msg, "read-only") {
		t.Errorf("w is off: %q", m.colors.msg)
	}
}

func TestCKeyWithoutAToolSaysSo(t *testing.T) {
	m := loaded(t, tabCards)
	m, _ = press(t, m, "C")
	if m.colors != nil || !strings.Contains(m.statusText(), "sitrep colors") {
		t.Errorf("%q", m.statusText())
	}
}

func TestCKeyIsTextInAPrompt(t *testing.T) {
	m, _ := colorsDash(t, false)
	m, _ = press(t, m, "/")
	m, _ = press(t, m, "C")
	if m.colors != nil || m.filter != "C" {
		t.Errorf("a typed C is part of the filter: %q", m.filter)
	}
}

func TestColorsMarksAreAboutTheFile(t *testing.T) {
	m, _ := newColors(t, "SITREP_COLORS=dim=7\n")
	row := func(m colorsModel, name string) string {
		for _, l := range strings.Split(ansi.Strip(m.body()), "\n") {
			if strings.Contains(l, " "+name+" ") && strings.Contains(l, "████") {
				return l
			}
		}
		t.Fatalf("no row for %s", name)
		return ""
	}
	if got := row(m, "dim"); !strings.HasSuffix(strings.TrimRight(got, " "), "set") {
		t.Errorf("a colour the file holds is marked set: %q", got)
	}
	if got := row(m, "hot"); strings.Contains(got, "*") || strings.Contains(got, "set") {
		t.Errorf("the theme's own colour has no mark: %q", got)
	}
	m = ckey(t, m, "j", "j", "j") // hot
	m = typeColor(t, m, "#445566")
	if got := row(m, "hot"); !strings.HasSuffix(strings.TrimRight(got, " "), "*") {
		t.Errorf("a change not written is marked *: %q", got)
	}
	m = ckey(t, m, "k", "r") // dim: reset, which is a change too
	if got := row(m, "dim"); !strings.HasSuffix(strings.TrimRight(got, " "), "*") {
		t.Errorf("a reset not written is marked *: %q", got)
	}
	m = ckey(t, m, "w", "y")
	if got := row(m, "hot"); !strings.HasSuffix(strings.TrimRight(got, " "), "set") {
		t.Errorf("after the write it is saved, so set: %q", got)
	}
	if got := row(m, "dim"); strings.Contains(got, "*") || strings.Contains(got, "set") {
		t.Errorf("a written reset is the theme's own again: %q", got)
	}
	m = ckey(t, m, "g", "l")
	if got := ansi.Strip(m.body()); !strings.Contains(got, "pastel  ") || !strings.Contains(got, "*") {
		t.Errorf("a theme not written is marked: %q", got)
	}
}

func lastLine(m colorsModel) string {
	lines := strings.Split(ansi.Strip(m.body()), "\n")
	return strings.TrimRight(lines[len(lines)-1], " ")
}

func TestColorsResultIsInTheFooter(t *testing.T) {
	m, _ := newColors(t, "A=1\n")
	m = ckey(t, m, "l", "w", "y")
	if got := lastLine(m); !strings.Contains(got, "written (backup sitrep.conf.bak.1)") {
		t.Errorf("the footer says what happened: %q", got)
	}
	m = ckey(t, m, "j")
	if got := lastLine(m); !strings.Contains(got, "j/k move") {
		t.Errorf("the next key brings the hints back: %q", got)
	}
	// a field that is refused says why where the eyes are, and the next key clears it
	m = ckey(t, m, "j")
	m = typeColor(t, m, "red")
	if got := lastLine(m); !strings.Contains(got, "not a colour") {
		t.Errorf("%q", got)
	}
	m = ckey(t, m, "a")
	if got := lastLine(m); strings.Contains(got, "not a colour") || !strings.Contains(got, "enter keeps") {
		t.Errorf("%q", got)
	}
}

func TestColorsNoBackupOfAnEmptyFile(t *testing.T) {
	m, f := newColors(t, "")
	m = ckey(t, m, "l", "w", "y")
	if len(f.backups) != 0 || !strings.Contains(m.msg, "no backup, the file was empty") || f.body != "SITREP_THEME=pastel\n" {
		t.Errorf("backups %q msg %q body %q", f.backups, m.msg, f.body)
	}
}

func TestColorsTooSmallSaysHowToQuit(t *testing.T) {
	m, _ := newColors(t, "")
	m.width, m.height = 60, 10
	if got := m.body(); !strings.Contains(got, "q quits") {
		t.Errorf("%q", got)
	}
}

func TestColorsThemeGallery(t *testing.T) {
	m, _ := newColors(t, "")
	m = ckey(t, m, "enter") // the theme row
	if m.gallery == nil || m.gallery.sel != 0 {
		t.Fatal("enter on the theme row opens the gallery")
	}
	body := ansi.Strip(m.body())
	for _, name := range ThemeNames {
		if !strings.Contains(body, name) {
			t.Errorf("the gallery lacks %s", name)
		}
	}
	if !strings.Contains(body, "needs you") || !strings.Contains(body, "✓") {
		t.Errorf("a sample of each, and the theme now marked:\n%s", body)
	}
	m = ckey(t, m, "j", "j")
	if m.theme != "default" || ThemeNames[m.gallery.sel] != "rainbow" {
		t.Errorf("moving changes nothing until enter: theme %q on %q", m.theme, ThemeNames[m.gallery.sel])
	}
	m = ckey(t, m, "esc")
	if m.gallery != nil || m.theme != "default" || m.dirty() {
		t.Error("esc leaves the theme as it was")
	}
	m = ckey(t, m, "enter", "G", "enter")
	if m.gallery != nil || m.theme != "mono" || !m.dirty() {
		t.Errorf("enter chooses: %q", m.theme)
	}
	m = ckey(t, m, "enter", "g", "j", "enter")
	if m.theme != ThemeNames[1] {
		t.Errorf("the gallery opens on the theme now and g goes to the first: %q", m.theme)
	}
}

func TestColorsGalleryShowsEachThemeInItsOwnColours(t *testing.T) {
	m, _ := newColors(t, "SITREP_COLORS=attn=#00ff00\n")
	m = ckey(t, m, "enter")
	raw := m.body()
	if strings.Contains(raw, "48;2;0;255;0") {
		t.Error("a colour you set is not part of a theme's sample")
	}
	lines := strings.Split(raw, "\n")
	if len(lines) != m.height || lines[1] == lines[2] {
		t.Errorf("one line each, in different colours: %d lines", len(lines))
	}
	before := raw
	m = ckey(t, m, "t")
	if m.body() == before {
		t.Error("t switches the samples between a dark and a light terminal")
	}
}

func TestColorsGalleryFitsTheSmallestScreen(t *testing.T) {
	m, _ := newColors(t, "")
	m = ckey(t, m, "enter")
	for _, w := range []int{20, 40, 80, 150} {
		m.width, m.height = w, minColorsHeight
		out := strings.Split(m.body(), "\n")
		if len(out) != minColorsHeight {
			t.Fatalf("%d wide: %d lines", w, len(out))
		}
		for _, l := range out {
			if ansi.StringWidth(l) > w {
				t.Fatalf("%d wide: %q", w, l)
			}
		}
		if !strings.Contains(ansi.Strip(m.body()), ThemeNames[len(ThemeNames)-1]) && w >= 40 {
			t.Errorf("%d wide: the last theme is cut off", w)
		}
	}
	golden(t, "colors_gallery_100x24", func() string { m.width, m.height = 100, 24; return ansi.Strip(m.body()) }())
}

func TestColorsWriteQuestionWrapsALongLineInsteadOfCuttingIt(t *testing.T) {
	long := "accent=#5b3fd1/#62aebf,dim=#8c8c8c/#8b8b96,hot=#1e3b45/#1e3b45,attn=#4a2a3a/#5c1f35,error=#e08a7a/#ff8a80,snoozed=#7fb3a0/#7aa2f7"
	m, _ := newColors(t, "SITREP_COLORS="+long+"\n")
	m.width, m.height = 60, 30
	m = ckey(t, m, "j", "j", "j", "j")
	m = typeColor(t, m, "#445566")
	m = ckey(t, m, "w")
	if m.ask == nil {
		t.Fatalf("no question: %q", m.msg)
	}
	q := ansi.Strip(m.body())
	for _, entry := range strings.Split(long, ",") {
		if entry != "attn=#4a2a3a/#5c1f35" && !strings.Contains(q, entry) {
			t.Errorf("the question cuts off %q:\n%s", entry, q)
		}
	}
	if strings.Contains(strings.SplitN(q, "\n", 2)[1], "…") {
		t.Errorf("nothing is cut:\n%s", q)
	}
	if !strings.Contains(q, "a is for a light terminal") {
		t.Errorf("the order of a/b is explained:\n%s", q)
	}
}
