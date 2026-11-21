// Package render draws the plain text listing that `sitrep ls` prints, from a snapshot: an Inbox, then a table of sessions with their
// Objective, Progress and Now, and optionally their queues. The text is the bash one (render_frame in bin/sitrep), without its height
// limit, wrapping and highlight, which belonged to the retired watch loop.
package render

import (
	"fmt"
	"strings"

	"github.com/lfeagan/claude-sitrep/internal/snapshot"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// Opts say how to draw.
type Opts struct {
	Verbose bool // add the Progress line, and list each queue when Items is above 0
	Items   int  // how many queue items to list
	Cols    int  // the width the Inbox and queue lines are cut to
}

// List says whether queues are listed, not just summarized.
func (o Opts) List() bool { return o.Verbose && o.Items > 0 }

// Cap is how many items of each queue the snapshot has to hold for these options.
func (o Opts) Cap() int {
	if o.List() {
		return o.Items
	}
	return 1 // the next item
}

// MaxLabelWidth is the widest the first column grows for an alias; a longer alias is cut with an ellipsis.
const MaxLabelWidth = 16

// idCell is a session's label cut to the column and padded to its width.
func idCell(label string, w int) string {
	return pad(textsafe.Fit(text(label), w), w)
}

// pad pads t with spaces to n columns.
func pad(t textsafe.Text, n int) string {
	return t.String() + strings.Repeat(" ", max(0, n-textsafe.Width(t)))
}

func text(s string) textsafe.Text { return textsafe.Clean(s, 0) }

// summary is "empty", "1 item" or "N items", and the next item unless only the count is wanted.
func summary(q snapshot.Queue, countOnly bool) string {
	var s string
	switch q.Count {
	case 0:
		s = "empty"
	case 1:
		s = "1 item"
	default:
		s = fmt.Sprintf("%d items", q.Count)
	}
	if q.Count > 0 && !countOnly && len(q.Items) > 0 {
		s += ", next: " + q.Items[0].String()
	}
	return s
}

// list is the first max items, numbered, then "+K more". The first line has the prefix first, the rest have other.
func list(q snapshot.Queue, max int, first, other string) []string {
	var out []string
	for i, it := range q.Items {
		if i >= max {
			break
		}
		p := other
		if i == 0 {
			p = first
		}
		out = append(out, fmt.Sprintf("%s%d. %s", p, i+1, it))
	}
	if q.Count > max {
		out = append(out, fmt.Sprintf("%s+%d more", other, q.Count-max))
	}
	return out
}

func fit(s string, cols int) string { return textsafe.Fit(text(s), cols).String() }

// Frame draws the listing, one string per line. It is empty when there is nothing to show.
func Frame(snap snapshot.Snapshot, o Opts) []string {
	dirW, stateW, idW := 9, 5, 4
	for _, s := range snap.Sessions {
		idW = min(max(idW, textsafe.Width(text(s.Label()))), MaxLabelWidth)
		stateW = max(stateW, textsafe.Width(text(s.StateLabel)))
		dirW = max(dirW, textsafe.Width(s.Dir))
	}
	var out []string
	if snap.Inbox.Count > 0 {
		if o.List() {
			out = append(out, fit("Inbox: "+summary(snap.Inbox, true), o.Cols))
			for _, l := range list(snap.Inbox, o.Items, "    ", "    ") {
				out = append(out, fit(l, o.Cols))
			}
		} else {
			out = append(out, fit("Inbox: "+summary(snap.Inbox, false), o.Cols))
		}
	}
	if len(snap.Sessions) > 0 {
		out = append(out, fmt.Sprintf("%s  %s  %s  %s", idCell("ID", idW), pad(text("DIRECTORY"), dirW), pad(text("STATE"), stateW), "QUEUE"))
	}
	avail := max(10, o.Cols-idW-dirW-stateW-6)
	for _, s := range snap.Sessions {
		out = append(out, fmt.Sprintf("%s  %s  %s  %s", idCell(s.Label(), idW), pad(s.Dir, dirW), pad(text(s.StateLabel), stateW), fit(summary(s.Queue, o.List()), avail)))
		out = append(out, "    Objective: "+s.Objective.String())
		if o.Verbose {
			out = append(out, "    Progress:  "+s.Progress.String())
		}
		out = append(out, "    Now:       "+s.Now.String())
		if o.List() {
			out = append(out, list(s.Queue, o.Items, "    Queue:     ", "               ")...)
		}
	}
	if snap.Hidden.Ended > 0 {
		out = append(out, fmt.Sprintf("+%d ended (--all)", snap.Hidden.Ended))
	}
	if snap.Hidden.StrandedItems > 0 {
		out = append(out, fmt.Sprintf("%d items stranded in ended sessions (sitrep prune)", snap.Hidden.StrandedItems))
	}
	return out
}
