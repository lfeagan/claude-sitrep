package conf

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/settings"
)

// Change is the new text of one setting of sitrep.conf, or its removal when Value is nil.
type Change struct {
	Name  string
	Value *string
}

// Editable are the settings Edit may change: the two that the colour tool owns.
var Editable = []string{"SITREP_THEME", "SITREP_COLORS"}

// ReadBody is the whole of sitrep.conf, as the colour tool edits it: a link is followed, a missing file is empty, and a file over 64 KB is
// refused.
func (c Conf) ReadBody() (string, error) { return c.readAll() }

// Edit makes the file text that follows from body and the changes, and says how many later lines of a changed name it dropped. A line is
// ours when it starts with NAME= at the first column, the test FromFile makes, so a commented, indented or `export`ed line is the user's and
// stays. The first line of a name is replaced where it stands and the later ones go, since the reader takes the first. A name with no line is
// added above the examples block, or at the end. A value the dashboard would not accept, or a name that is not Editable, is an error.
// Everything else is kept byte for byte, and a file of \r\n lines gets \r\n lines.
func Edit(body string, changes []Change) (out string, dropped int, err error) {
	eol := "\n"
	if strings.Contains(body, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.SplitAfter(body, "\n") // every line keeps its own ending; the last may have none
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for _, ch := range changes {
		if err := check(ch); err != nil {
			return "", 0, err
		}
		prefix := ch.Name + "="
		var kept []string
		done := false
		for _, l := range lines {
			if !strings.HasPrefix(l, prefix) {
				kept = append(kept, l)
				continue
			}
			if done {
				dropped++
				continue
			}
			done = true
			if ch.Value != nil {
				kept = append(kept, line(ch, endingOf(l)))
			}
		}
		lines = kept
		if !done && ch.Value != nil {
			lines = insert(lines, line(ch, eol), eol)
		}
	}
	return strings.Join(lines, ""), dropped, nil
}

func check(ch Change) error {
	ok := false
	for _, n := range Editable {
		ok = ok || n == ch.Name
	}
	switch {
	case !ok:
		return fmt.Errorf("%s is not a setting the colour tool changes", ch.Name)
	case ch.Value == nil:
		return nil
	case strings.ContainsAny(*ch.Value, "\r\n"):
		return errors.New("a setting is one line")
	case ch.Name == "SITREP_THEME" && !ValidTheme(*ch.Value):
		return fmt.Errorf("%q is not a theme name", *ch.Value)
	case ch.Name == "SITREP_COLORS" && !ValidColors(*ch.Value):
		return errors.New("the colours are too long, or have a character the dashboard does not read")
	}
	return nil
}

// endingOf is the line ending a line has, which its replacement keeps: a file may mix \n and \r\n, and a last line may have none.
func endingOf(l string) string {
	switch {
	case strings.HasSuffix(l, "\r\n"):
		return "\r\n"
	case strings.HasSuffix(l, "\n"):
		return "\n"
	}
	return ""
}

// line is the setting as a line; a theme name is bare letters, and the colours are quoted for their spaces and commas.
func line(ch Change, eol string) string {
	v := *ch.Value
	if ch.Name != "SITREP_THEME" {
		v = Quote(v)
	}
	return ch.Name + "=" + v + eol
}

// insert puts l above the examples block (and the blank lines before it), or at the end of the file.
func insert(lines []string, l, eol string) []string {
	at := len(lines)
	for i, x := range lines {
		if strings.HasPrefix(x, examplesBegin) {
			at = i
			for at > 0 && strings.TrimRight(lines[at-1], "\r\n") == "" {
				at--
			}
			break
		}
	}
	if at == len(lines) && at > 0 && !strings.HasSuffix(lines[at-1], "\n") {
		lines[at-1] += eol
	}
	out := append([]string{}, lines[:at]...)
	out = append(out, l)
	return append(out, lines[at:]...)
}

// Backup saves body, as it was read, as sitrep.conf.bak.<time> beside sitrep.conf (the state folder, not the folder a link leads to, so a
// dotfiles repository does not collect backups): owner-only, never over an earlier backup.
func (c Conf) Backup(body string, now time.Time) (string, error) {
	return settings.Backup(c.File, []byte(body), now)
}

// WriteBody replaces the file with out when it still holds original, as the settings editor does (settings.ErrChanged when it does not).
func (c Conf) WriteBody(out, original string) error { return c.write(out, []byte(original)) }

// OtherLines is body without the lines Edit may change, to check that a write changed nothing else.
func OtherLines(body string) string {
	var keep []string
	for _, l := range strings.SplitAfter(body, "\n") {
		own := false
		for _, n := range Editable {
			own = own || strings.HasPrefix(l, n+"=")
		}
		if !own {
			keep = append(keep, l)
		}
	}
	return strings.TrimRight(strings.Join(keep, ""), "\r\n") // a write may add the ending the last line lacked
}

// Line is the first line of body that sets name, as it stands, or "".
func Line(body, name string) string {
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, name+"=") {
			return strings.TrimRight(l, "\r")
		}
	}
	return ""
}

// Target is the file a write goes to: sitrep.conf, or what a link at that path leads to.
func (c Conf) Target() string {
	if t, err := filepath.EvalSymlinks(c.File); err == nil {
		return t
	}
	return c.File
}
