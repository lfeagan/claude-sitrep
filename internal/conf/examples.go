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
# The lines of the status line's block: any of objective, progress, now and queue, separated by commas. The default is all four.
# The inbox count is part of the queue line. A value with no known name shows all four.
# SITREP_STATUS_LINES=objective,progress,now,queue
#
# Seconds a session that needs you keeps its needs-you colour, counted from when it began to wait. After that it is
# drawn plain, though it still counts as needing you (the dot, the count and the bell are the same). 0 never fades.
# SITREP_ATTN_FADE=600
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
# The dashboard's colours: a theme, and changes to single colours. The themes are default (purple, with a
# pink-red for what needs you), pastel (soft lavender, mint and peach), rainbow (a colour of the spectrum
# for each role: green accent, blue text, orange for recent activity, yellow for what needs you, red for
# errors), high-contrast, mono (no colour), and ocean, forest, homebrew, red-sands, novel, nord, solarized, winter,
# spring, summer and fall (see sitrep colors, which shows each).
# A colour is #rrggbb or a terminal colour number from 0 to 255, or light/dark for each kind of terminal.
# The names are accent, dim, hot (recent activity), attn (needs you), error and snoozed (the ○ mark).
# SITREP_THEME=default
# SITREP_COLORS='accent=#d9480f,attn=#c8f0c8/#52b35d,hot=236,dim=244,error=#b3261e'
#
# How often the dashboard refreshes: seconds, or 500ms, 5s, 1m. At least one second.
# SITREP_TUI_INTERVAL=3
#
# The dashboard without any key that changes state, and without the bell when a session needs you.
# SITREP_TUI_READONLY=1
# SITREP_TUI_BELL=0
#
# A click on a row selects it and a double click jumps to its terminal, as enter does. The terminal then
# selects text in the dashboard only with shift (or option in iTerm2 and Terminal.app) held. Off by default.
# SITREP_TUI_MOUSE=1
#
# Tell Claude how many items this session's queue holds, so that it asks you whether to start the next one when a task is done.
# It never starts one on its own, and the line holds the count and no item text. Off by default.
# SITREP_QUEUE_NUDGE=on
#
# A history of every change sitrep makes (queues, pins, aliases), in the state folder. Off by default.
# It keeps the text of items. Read it with 'sitrep log'; 'sitrep log --clear' deletes it.
# SITREP_LOG=on
# The most megabytes it may hold, and the most days an entry is kept. Whichever is reached first wins.
# SITREP_LOG_MAX=10
# SITREP_LOG_DAYS=30
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
	b, err := state.ReadFileFollow(c.File, 64<<10+1)
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
	return c.write(out, []byte(body))
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
	return c.write(rest+"\n", []byte(body))
}
