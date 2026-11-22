package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/lfeagan/claude-sitrep/internal/conf"
	"github.com/lfeagan/claude-sitrep/internal/settings"
	"github.com/lfeagan/claude-sitrep/internal/snapshot"
)

// ColorsFile is the file the colour tool reads and writes, which is sitrep.conf.
type ColorsFile interface {
	Path() string                     // the file a write goes to, with a link followed
	Values() (theme, colors string)   // what the file holds, whatever the environment says
	Read() (string, error)            // the whole file
	Write(out, original string) error // replace it, when it still holds original
	Backup(body string) (string, error)
}

// ColorsOptions are the colour tool's inputs.
type ColorsOptions struct {
	Sessions snapshot.Snapshot // what the preview shows
	File     ColorsFile
	ReadOnly bool
	Dark     *bool    // the kind of terminal to preview, until the terminal says which it is
	Notes    []string // what to say at the start: an environment setting that beats the file, NO_COLOR
}

const minColorsHeight = 18

// colorRows are the rows of the list: the theme, then each colour.
var colorRows = append([]string{"theme"}, colorNames...)

type colorEdit struct {
	name string
	text string
	lch  oklch  // where the steps have taken the colour, which may be more than the screen can show
	from string // the text lch was made for: when the text is not this, it was typed, and lch is made again
}

type colorAsk struct {
	kind     string // "write" or "quit"
	body     string // what the file held when the question was made
	out      string // what it would hold
	old, new []string
	dropped  int
}

type colorsModel struct {
	opts          ColorsOptions
	snap          Snapshot
	theme         string
	set           map[string][2]string // the colours the user has set: light, dark
	dark          bool
	sel           int
	edit          *colorEdit
	ask           *colorAsk
	msg           string
	width, height int
	backedUp      string // the bytes last saved as a backup
	lastBackup    string
	savedTheme    string
	savedColors   string
}

func newColorsModel(opts ColorsOptions) colorsModel {
	theme, colors := opts.File.Values()
	m := colorsModel{opts: opts, snap: FromSnapshot(opts.Sessions), dark: true, width: 100, height: 30, set: parseColors(colors)}
	m.theme = strings.ToLower(theme)
	if _, ok := presets[m.theme]; !ok && m.theme != "mono" {
		m.theme = "default"
	}
	if opts.Dark != nil {
		m.dark = *opts.Dark
	}
	m.savedTheme, m.savedColors = m.theme, m.colorsString()
	m.msg = strings.Join(opts.Notes, "  ")
	return m
}

// value is the colour a row has: the one the user set, or the theme's own.
func (m colorsModel) value(name string) [2]string {
	if v, ok := m.set[name]; ok {
		return v
	}
	return themeValue(m.theme, name)
}

func themeValue(theme, name string) [2]string {
	p, ok := presets[theme]
	if !ok {
		p = presets["default"]
	}
	switch name {
	case "accent":
		return p.accent
	case "dim":
		return p.dim
	case "hot":
		return p.hot
	case "attn":
		return p.attn
	case "error":
		return p.errc
	}
	return p.snooze
}

func (m colorsModel) side() int {
	if m.dark {
		return 1
	}
	return 0
}

// colorsString is SITREP_COLORS as the file would hold it: the colours that are set, one value when both kinds of terminal share it.
func (m colorsModel) colorsString() string { return colorsOf(m.set) }

func colorsOf(set map[string][2]string) string {
	var parts []string
	for _, n := range colorNames {
		if v, ok := set[n]; ok {
			if v[0] == v[1] {
				parts = append(parts, n+"="+v[0])
			} else {
				parts = append(parts, n+"="+v[0]+"/"+v[1])
			}
		}
	}
	return strings.Join(parts, ",")
}

func (m colorsModel) dirty() bool {
	return m.theme != m.savedTheme || m.colorsString() != m.savedColors
}

// changes are the two settings as they should stand in the file.
func (m colorsModel) changes() []conf.Change {
	var out []conf.Change
	if m.theme != m.savedTheme { // a setting the user did not change is left as the file has it
		var theme *string
		if m.theme != "default" {
			theme = &m.theme
		}
		out = append(out, conf.Change{Name: "SITREP_THEME", Value: theme})
	}
	if s := m.colorsString(); s != m.savedColors {
		var colors *string
		if s != "" {
			colors = &s
		}
		out = append(out, conf.Change{Name: "SITREP_COLORS", Value: colors})
	}
	return out
}

// previewTheme is the theme as it would be, with the colour being edited at the value in the field.
func (m colorsModel) previewTheme() Theme {
	set := m.set
	if m.edit != nil {
		if c := normColor(m.edit.text); c != "" {
			v := m.value(m.edit.name)
			v[m.side()] = c
			set = cloneSet(m.set)
			set[m.edit.name] = v
		}
	}
	return themeFor(m.dark, Settings{Theme: m.theme, Colors: colorsOf(set)})
}

func (m colorsModel) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (m colorsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(msg.Width, 1), max(msg.Height, 1)
	case tea.BackgroundColorMsg:
		if m.opts.Dark == nil {
			m.dark = msg.IsDark()
		}
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m colorsModel) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}
	switch {
	case m.ask != nil:
		return m.askKey(k)
	case m.edit != nil:
		return m.editKey(msg)
	}
	m.msg = ""
	switch k {
	case "q", "esc":
		if m.dirty() {
			m.ask = &colorAsk{kind: "quit"}
			return m, nil
		}
		return m, tea.Quit
	case "j", "down":
		m.sel = min(m.sel+1, len(colorRows)-1)
	case "k", "up":
		m.sel = max(m.sel-1, 0)
	case "g", "home":
		m.sel = 0
	case "G", "end":
		m.sel = len(colorRows) - 1
	case "t":
		m.dark = !m.dark
	case "h", "left":
		if m.sel == 0 {
			m.cycleTheme(-1)
		}
	case "l", "right":
		if m.sel == 0 {
			m.cycleTheme(1)
		}
	case "enter":
		return m.open()
	case "r":
		if m.sel == 0 {
			m.theme = "default"
		} else if _, ok := m.set[colorRows[m.sel]]; ok {
			m.set = cloneSet(m.set)
			delete(m.set, colorRows[m.sel])
		}
	case "w":
		m.beginWrite()
	}
	return m, nil
}

func cloneSet(s map[string][2]string) map[string][2]string {
	out := make(map[string][2]string, len(s)+1)
	for k, v := range s {
		out[k] = v
	}
	return out
}

func (m *colorsModel) cycleTheme(d int) {
	i := 0
	for j, n := range ThemeNames {
		if n == m.theme {
			i = j
		}
	}
	m.theme = ThemeNames[(i+d+len(ThemeNames))%len(ThemeNames)]
}

// open starts editing the selected colour, or moves the theme on.
func (m colorsModel) open() (tea.Model, tea.Cmd) {
	if m.sel == 0 {
		m.cycleTheme(1)
		return m, nil
	}
	if m.theme == "mono" {
		m.msg = "mono has no colours: pick another theme first"
		return m, nil
	}
	name := colorRows[m.sel]
	m.edit = &colorEdit{name: name, text: m.value(name)[m.side()]}
	return m, nil
}

func (m colorsModel) editKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	e := *m.edit
	switch k := msg.String(); k {
	case "esc":
		m.edit = nil
		return m, nil
	case "enter":
		c := normColor(e.text)
		if c == "" {
			m.msg = "not a colour: use #rrggbb or a number from 0 to 255"
			return m, nil
		}
		v := m.value(e.name)
		v[m.side()] = c
		m.set = cloneSet(m.set)
		m.set[e.name] = v
		m.edit = nil
		return m, nil
	case "up", "down":
		if n, ok := colorNumber(e.text); ok { // a terminal colour number: walk the 256
			d := 1
			if k == "up" {
				d = -1
			}
			e.text = fmt.Sprint(min(max(n+d, 0), 255))
		} else {
			e.step(0, map[string]int{"up": 1, "down": -1}[k], 0)
		}
	case "h", "l", "left", "right", "j", "k", "H", "L":
		hue, light, chroma := 0, 0, 0
		switch k {
		case "h", "left":
			hue = -1
		case "l", "right":
			hue = 1
		case "j":
			light = -1
		case "k":
			light = 1
		case "H":
			chroma = -1
		case "L":
			chroma = 1
		}
		e.step(hue, light, chroma)
	case "backspace":
		if r := []rune(e.text); len(r) > 0 {
			e.text = string(r[:len(r)-1])
		}
	case "ctrl+u":
		e.text = ""
	case "ctrl+w":
		e.text = dropWord(e.text)
	default:
		if msg.Text != "" && !strings.HasPrefix(k, "ctrl+") && !strings.HasPrefix(k, "alt+") && len([]rune(e.text)) < 7 {
			e.text += strings.Map(func(r rune) rune {
				if r < ' ' || r > '~' {
					return -1
				}
				return r
			}, msg.Text)
		}
	}
	m.edit = &e
	return m, nil
}

// step moves the colour in the field, when the field holds one.
func (e *colorEdit) step(hue, light, chroma int) {
	c := normColor(e.text)
	if c == "" {
		return
	}
	if e.from != e.text {
		e.lch = hexToOKLCH(c)
	}
	e.lch = e.lch.stepped(hue, light, chroma)
	e.text = e.lch.hex()
	e.from = e.text
}

func colorNumber(s string) (int, bool) {
	c := normColor(s)
	if c == "" || c[0] == '#' {
		return 0, false
	}
	var n int
	fmt.Sscan(c, &n)
	return n, true
}

// beginWrite makes the question that says what would change in the file.
func (m *colorsModel) beginWrite() {
	switch {
	case m.opts.ReadOnly:
		m.msg = "read-only: nothing is written"
		return
	case !m.dirty():
		m.msg = "nothing to write: the file already holds this"
		return
	}
	body, err := m.opts.File.Read()
	if err != nil {
		m.msg = "cannot read " + m.opts.File.Path() + ": " + err.Error()
		return
	}
	m.ask = nil
	out, dropped, err := conf.Edit(body, m.changes())
	if err != nil {
		m.msg = err.Error()
		return
	}
	if out == body {
		m.msg = "nothing to write: the file already holds this"
		return
	}
	a := &colorAsk{kind: "write", body: body, out: out, dropped: dropped}
	for _, n := range conf.Editable {
		a.old = append(a.old, orNone(conf.Line(body, n)))
		a.new = append(a.new, orNone(conf.Line(out, n)))
	}
	m.ask = a
}

// safe is text from the file or the system, cleaned and cut before it is drawn: the lines of sitrep.conf are not ours.
func safe(s string) string {
	s = clean(s)
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300]) + "…"
	}
	return s
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func (m colorsModel) askKey(k string) (tea.Model, tea.Cmd) {
	a := m.ask
	switch k {
	case "y":
		if a.kind == "quit" {
			return m, tea.Quit
		}
		return m.write()
	case "n", "esc", "q":
		m.ask = nil
	}
	return m, nil
}

// write saves the lines: from the file as it is now, with a backup first, and only over the bytes the new text was made from.
func (m colorsModel) write() (tea.Model, tea.Cmd) {
	a := m.ask
	m.ask = nil
	f := m.opts.File
	fresh, err := f.Read()
	if err != nil {
		m.msg = "cannot read " + f.Path() + ": " + err.Error()
		return m, nil
	}
	if fresh != a.body { // the file was edited while the question was open: ask again about what it holds now
		m.beginWrite()
		if m.ask != nil {
			m.msg = "the file changed while you looked: check the lines again"
		}
		return m, nil
	}
	backup := ""
	if fresh != m.backedUp || m.lastBackup == "" { // each write saves the bytes it is about to replace, once
		if backup, err = f.Backup(fresh); err != nil {
			m.msg = "nothing written, no backup could be made: " + err.Error()
			return m, nil
		}
		m.backedUp, m.lastBackup = fresh, backup
	}
	backup = m.lastBackup
	if err := f.Write(a.out, fresh); err != nil {
		if errors.Is(err, settings.ErrChanged) {
			m.msg = "the file changed just now: nothing was written, try again"
		} else {
			m.msg = "nothing written: " + err.Error()
		}
		return m, nil
	}
	got, err := f.Read()
	ours := true
	for _, n := range conf.Editable {
		ours = ours && conf.Line(got, n) == conf.Line(a.out, n)
	}
	switch {
	case err != nil:
		m.msg = "written, but the file could not be read back to check it: the backup is " + backup
	case conf.OtherLines(got) != conf.OtherLines(fresh) || !ours:
		m.msg = "written, but the file is not as it should be: check it, the backup is " + backup
	default:
		m.msg = "written; sitrep dash shows it at its next refresh (backup: " + backup + ")"
	}
	m.savedTheme, m.savedColors = m.theme, m.colorsString()
	return m, nil
}

func (m colorsModel) View() tea.View {
	v := tea.NewView(m.body())
	v.AltScreen = true
	return v
}

func (m colorsModel) body() string {
	w, h := max(m.width, 1), m.height
	if h < minColorsHeight {
		return ansi.Truncate(fmt.Sprintf("make the window taller (%d rows) to choose colours", minColorsHeight), w, "…")
	}
	kind := "light"
	if m.dark {
		kind = "dark"
	}
	head := lipgloss.NewStyle().Bold(true).Render(ansi.Truncate(fmt.Sprintf(" Colours   theme: %s   preview: %s terminal (t switches)", m.theme, kind), w, "…"))
	note := ansi.Truncate(" "+safe(m.msg), w, "…")

	list := m.list(w)
	room := h - 3 - len(list) // the header, the note and the footer
	var middle []string
	if m.ask != nil && m.ask.kind == "write" {
		middle = m.writeQuestion(room, w)
	} else {
		middle = strings.Split(render(m.snap, tabTable, "error: a sample error (showing data from 12s ago)", viewOpts{Width: w, Height: room, Theme: m.previewTheme()}), "\n")
	}
	for len(middle) < room {
		middle = append(middle, "")
	}
	out := append([]string{head, note}, middle[:room]...)
	out = append(out, list...)
	out = append(out, m.footer(w))
	return strings.Join(out, "\n")
}

func (m colorsModel) list(w int) []string {
	mono := m.theme == "mono"
	dim := lipgloss.NewStyle().Faint(true)
	var out []string
	for i, name := range colorRows {
		cur := "  "
		if i == m.sel {
			cur = "▶ "
		}
		val, star, swatch := m.theme, "", ""
		if i > 0 {
			c := m.value(name)[m.side()]
			val = c
			if _, ok := m.set[name]; ok {
				star = " *"
			}
			if !mono {
				swatch = " " + lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render("████") + " " + lipgloss.NewStyle().Background(lipgloss.Color(c)).Render("    ")
			}
		}
		line := fmt.Sprintf("%s%-8s %-9s%s%s", cur, name, val, swatch, star)
		if mono && i > 0 {
			line = dim.Render(fmt.Sprintf("%s%-8s (mono has no colours)", cur, name))
		}
		out = append(out, ansi.Truncate(line, w, "…"))
	}
	return out
}

func (m colorsModel) footer(w int) string {
	bar := lipgloss.NewStyle().Reverse(true).Width(w)
	var text string
	switch {
	case m.ask != nil && m.ask.kind == "quit":
		text = " quit without writing?  y/n"
	case m.ask != nil:
		text = " y write these lines  n cancel"
	case m.edit != nil:
		text = fmt.Sprintf(" %s (%s terminal): %s▏  h/l hue  j/k light  H/L chroma  enter keeps  esc cancels", m.edit.name, map[bool]string{true: "dark", false: "light"}[m.dark], m.edit.text)
	default:
		text = " j/k move  enter edit  r reset  t light/dark  w write  q quit"
		if m.sel == 0 {
			text = " j/k move  h/l theme  r reset  t light/dark  w write  q quit"
		}
	}
	return bar.Render(ansi.Truncate(text, w, ""))
}

// writeQuestion shows the lines that would change in the file.
func (m colorsModel) writeQuestion(room, w int) []string {
	a := m.ask
	out := []string{" Write to " + safe(m.opts.File.Path()) + ":"}
	for i := range conf.Editable {
		if a.old[i] != a.new[i] {
			out = append(out, "   was  "+safe(a.old[i]), "   now  "+safe(a.new[i]))
		}
	}
	if a.dropped > 0 {
		out = append(out, fmt.Sprintf(" %d later line(s) of the same setting are removed: the first one counts", a.dropped))
	}
	out = append(out, " Everything else stays as it is. A backup is made first.")
	for i, l := range out {
		out[i] = ansi.Truncate(l, w, "…")
	}
	return out
}
