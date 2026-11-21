package conf

import (
	"errors"
	"os"
	"strings"

	"github.com/lfeagan/claude-sitrep/internal/state"
)

// The block of commented examples that setup keeps at the end of sitrep.conf. Everything between the two lines is ours and is replaced as a
// whole; the user's own lines stay above it, or anywhere else.
const (
	examplesBegin = "# >>> sitrep settings: examples (setup keeps this block up to date; put your own settings above it) >>>"
	examplesEnd   = "# <<< sitrep settings <<<"
)

// Examples is the text of the block: every setting, commented out, with its default.
const Examples = `# Uncomment a line and change it to use it. The environment wins over this file.
# This file is read and never run: NAME=value lines only, in single quotes when a value has spaces or commas.
# SITREP_HUD_CMD is written by setup, to keep the status line that was there before. Leave it alone.
#
# How many queue items a listing shows.
# SITREP_QUEUE_ITEMS=5
#
# Seconds a session counts as recently active (the dashboard colours it). 0 turns it off.
# SITREP_HIGHLIGHT_SECONDS=60
#
# That colour as terminal SGR codes (digits and semicolons only). SITREP_COLORS below wins over it.
# SITREP_HIGHLIGHT_SGR=48;5;238
#
# Seconds between summaries of a session. A value that is not a number turns the wait off.
# SITREP_DEBOUNCE=45
#
# Look once a day for a newer release. 0, false or no turns it off.
# SITREP_CHECK_UPDATES=1
#
# The dashboard's colours: a theme (default, high-contrast or mono), and changes to single colours.
# A colour is #rrggbb or a terminal colour number from 0 to 255, or light/dark for each kind of terminal.
# The names are accent, dim, hot (recent activity), attn (needs you) and error.
# SITREP_THEME=default
# SITREP_COLORS='accent=#d9480f,attn=#c8f0c8/#52b35d,hot=236,dim=244,error=#b3261e'
#
# How often the dashboard refreshes: seconds, or 500ms, 5s, 1m. At least one second.
# SITREP_TUI_INTERVAL=3
#
# The dashboard without any key that changes state, and without the bell when a session needs you.
# SITREP_TUI_READONLY=1
# SITREP_TUI_BELL=0
`

// block is Examples with its two marker lines.
func block() string { return examplesBegin + "\n" + Examples + examplesEnd + "\n" }

// without is body with the examples block taken out, and whether there was one. The lines around it are joined by one blank line, or none
// when the block was first or last.
func without(body string) (rest string, found bool) {
	i := strings.Index(body, examplesBegin)
	if i < 0 {
		return body, false
	}
	j := strings.Index(body[i:], examplesEnd)
	if j < 0 {
		return body, false // a block that was cut short is left to the user
	}
	head, tail := strings.TrimRight(body[:i], "\n"), strings.TrimLeft(body[i+j+len(examplesEnd):], "\n")
	if head != "" && tail != "" {
		return head + "\n\n" + tail, true
	}
	return head + tail, true
}

func (c Conf) readAll() (string, error) {
	b, err := state.ReadFile(c.File, 64<<10+1)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if len(b) > 64<<10 {
		return "", errors.New("sitrep.conf is too large to edit")
	}
	return string(b), nil
}

// PutExamples makes the examples block the last thing in the file, replacing an earlier copy of it. The user's lines are kept as they are,
// and nothing is written when the file already ends with the current block.
func (c Conf) PutExamples() error {
	body, err := c.readAll()
	if err != nil {
		return err
	}
	rest, _ := without(body)
	rest = strings.TrimRight(rest, "\n")
	if rest != "" {
		rest += "\n\n"
	}
	out := rest + block()
	if out == body {
		return nil
	}
	return c.store().WriteFile(c.File, []byte(out))
}

// DropExamples removes the block. The file goes when nothing else is left.
func (c Conf) DropExamples() error {
	body, err := c.readAll()
	if err != nil {
		return err
	}
	rest, found := without(body)
	if !found {
		return nil
	}
	rest = strings.TrimRight(rest, "\n")
	if strings.TrimSpace(rest) == "" {
		return os.Remove(c.File)
	}
	return c.store().WriteFile(c.File, []byte(rest+"\n"))
}
