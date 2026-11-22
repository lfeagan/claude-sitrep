package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// The Wall tab lays the sessions out as tiles of one height, in as many columns as the width holds (docs/WALL.md).

// Tile sizes, cycled with s. The zero value is the standard size.
const (
	wallStandard = iota
	wallRoomy
	wallCompact
	wallSizes
)

var wallSizeNames = [wallSizes]string{"standard", "roomy", "compact"}

// wallRows is the height of a tile of each size.
var wallRows = [wallSizes]int{4, 6, 2}

// wallMinTile is the narrowest a tile is made, so the number of columns is the width divided by it.
const wallMinTile = 30

// wallCell is where one tile is on the body, for a mouse click: its columns and its rows, both ends included.
type wallCell struct {
	X0, X1, Y0, Y1 int
	ID             string
}

// wallCols is the number of tiles in a row on a screen of the given width.
func wallCols(width int) int { return max(width/wallMinTile, 1) }

// wallGap is the blank rows between two rows of tiles: none for the compact size.
func wallGap(size int) int {
	if size == wallCompact {
		return 0
	}
	return 1
}

// wallPage is how many sessions ctrl+d and ctrl+u move on a screen of this size: the tile rows that fit, as many sessions as that.
func wallPage(width, height, size int) int {
	pitch := wallRows[size] + wallGap(size)
	rows := max((height-5+wallGap(size))/pitch, 1)
	return rows * wallCols(width)
}

// tileRows is the text of one tile, one string per row and each cut to cw columns. Every size has the same rows for every session.
func tileRows(x Session, size, cw int) []string {
	head := x.Label() + "  " + stateText(x)
	now := x.Now
	if now == "" {
		now = x.Objective
	}
	if now == "" {
		now = "-"
	}
	queue := "no queue"
	if x.Queue.Count > 0 {
		queue = fmt.Sprintf("%d queued", x.Queue.Count)
		if len(x.Queue.Items) > 0 {
			queue += ": " + x.Queue.Items[0]
		}
	}
	dir := x.Dir
	if x.Context != nil && x.Running {
		left := min(max(x.Context.Left(), 0), 100)
		ctx := fmt.Sprintf("%d%%", left)
		if left <= 10 {
			ctx = "!" + ctx
		}
		if w := cw - ansi.StringWidth(ctx) - 1; w >= 8 {
			dir = padRight(ansi.Truncate(dir, w, "…"), w) + " " + ctx
		}
	}
	var rows []string
	switch size {
	case wallCompact:
		if x.Queue.Count > 0 {
			head += fmt.Sprintf("  +%d", x.Queue.Count)
		}
		rows = []string{head, now}
	case wallRoomy:
		rows = []string{head, orDash(x.Objective), orDash(x.Progress), now, queue, dir}
	default:
		rows = []string{head, now, queue, dir}
	}
	for i, r := range rows {
		rows[i] = ansi.Truncate(r, max(cw, 1), "…")
	}
	return rows
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// wallBody is the tab: a summary line, the tiles in rows, and the footers. It scrolls by tile row so that the selected tile stays on
// screen, and says how many tiles are above and below. o.Cells, when set, gets the place of every tile that is drawn.
func wallBody(s Snapshot, o viewOpts) []string {
	list, ended := shown(s, o.ShowEnded)
	list = sorted(filterSessions(list, o.Filter), sortState)
	head := summary(s)
	if s.Inbox.Count > 0 {
		head += "   inbox " + queueWord(s.Inbox.Count)
	}
	head += "   size: " + wallSizeNames[o.Size%wallSizes]
	if len(list) == 0 && strings.TrimSpace(o.Filter) != "" {
		head += "   " + noMatch(s, o)
	}
	out := []string{head}
	foot := 0
	if ended > 0 {
		foot++
	}
	if s.Hidden.StrandedItems > 0 {
		foot++
	}

	size := o.Size % wallSizes
	rows, gap := wallRows[size], wallGap(size)
	cols := wallCols(o.Width)
	tw := max(o.Width/cols, 1) // a tile with its gap
	cw := max(tw-3, 1)         // the text of a tile: its gutter and its gap are not in it
	n := len(list)
	nRows := (n + cols - 1) / cols
	pitch := rows + gap

	selRow := 0
	if i := indexOf(list, o.Sel); i >= 0 {
		selRow = i / cols
	}
	first, shown := 0, nRows
	if o.Height > 0 && nRows > 0 {
		avail := o.Height - len(out) - foot
		fit := func(a int) int { return max((a+gap)/pitch, 1) }
		if nRows*pitch-gap > avail {
			for first = 0; ; first++ {
				up := 0
				if first > 0 {
					up = 1
				}
				shown = fit(avail - up - 1) // room for the line that counts the tiles below
				if first+shown >= nRows {
					shown = min(fit(avail-up), nRows-first)
				}
				if selRow < first+shown || first >= nRows-1 {
					break
				}
			}
		}
	}
	if first > 0 {
		out = append(out, fmt.Sprintf("+%d above", first*cols))
	}
	tileOpts := o
	tileOpts.Width = tw - 1
	for r := first; r < first+shown && r < nRows; r++ {
		if r > first && gap > 0 {
			out = append(out, "")
		}
		base := len(out)
		tiles := make([][]string, 0, cols)
		for c := 0; c < cols; c++ {
			i := r*cols + c
			if i >= n {
				break
			}
			x := list[i]
			lines := make([]string, rows)
			for k, text := range tileRows(x, size, cw) {
				lines[k] = padRight(gutter(x.ID, o)+text, tw-1)
			}
			lines = selectBlock(mark(lines, x, tileOpts), x, tileOpts)
			tiles = append(tiles, lines)
			if o.Cells != nil {
				*o.Cells = append(*o.Cells, wallCell{X0: c * tw, X1: max(c*tw, c*tw+tw-2), Y0: base, Y1: base + rows - 1, ID: x.ID})
			}
		}
		for k := 0; k < rows; k++ {
			parts := make([]string, cols)
			for c := range parts {
				if c < len(tiles) {
					parts[c] = tiles[c][k]
				} else {
					parts[c] = strings.Repeat(" ", tw-1)
				}
			}
			out = append(out, strings.Join(parts, " "))
		}
	}
	if below := n - (first+shown)*cols; below > 0 && first+shown < nRows {
		out = append(out, fmt.Sprintf("+%d more", below))
	}
	if ended > 0 {
		out = append(out, fmt.Sprintf("+%d ended (a)", ended))
	}
	if s.Hidden.StrandedItems > 0 {
		out = append(out, fmt.Sprintf("%d items stranded in ended sessions (sitrep prune)", s.Hidden.StrandedItems))
	}
	if o.Height > 0 && len(out) > o.Height {
		out = out[:o.Height]
	}
	return out
}
