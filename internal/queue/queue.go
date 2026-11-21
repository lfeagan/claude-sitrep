// Package queue holds the work queues: one file per session and one for the inbox, a line per item. Its functions return typed errors, so
// the command line and the dashboard can say the same thing in their own way.
package queue

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// MaxItem is the longest item, in characters.
const MaxItem = 200

// Label is the short name of a queue: "inbox", or the first four characters of a session id.
func Label(id state.ID) string {
	if id == state.Inbox || len(id) <= 4 {
		return string(id)
	}
	return string(id[:4])
}

// ChangedError says the queue is no longer what the caller saw.
type ChangedError struct{ ID state.ID }

func (e *ChangedError) Error() string {
	return fmt.Sprintf("queue changed; run 'sitrep ls %s'", Label(e.ID))
}

// NoItemError says there is no item at that position.
type NoItemError struct {
	ID state.ID
	N  int
}

func (e *NoItemError) Error() string { return fmt.Sprintf("no item %d on %s", e.N, Label(e.ID)) }

// RangeError says a position is past the end of the queue.
type RangeError struct {
	ID    state.ID
	Count int
}

func (e *RangeError) Error() string { return fmt.Sprintf("%s has %d items", Label(e.ID), e.Count) }

// ErrEmpty says the queue has no items.
var ErrEmpty = errors.New("queue empty")

// ErrBlank says the text to queue holds nothing.
var ErrBlank = errors.New("nothing to queue")

// Queue is the set of queues in a state folder.
type Queue struct{ Store state.Store }

func (q Queue) file(id state.ID) string { return q.Store.Path(id, "queue") }

// lines reads the raw lines of a queue; a missing file is an empty queue.
func (q Queue) lines(id state.ID) ([]string, error) {
	b, err := os.ReadFile(q.file(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	s := strings.TrimSuffix(string(b), "\n")
	if s == "" && len(b) == 0 {
		return nil, nil
	}
	return strings.Split(s, "\n"), nil
}

func (q Queue) write(id state.ID, lines []string) error {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return q.Store.WriteFile(q.file(id), []byte(b.String()))
}

// Count is the number of non-empty lines.
func (q Queue) Count(id state.ID) int {
	lines, _ := q.lines(id)
	n := 0
	for _, l := range lines {
		if l != "" {
			n++
		}
	}
	return n
}

// same compares two items the way the JSON seen by a program does: trailing spaces do not count.
func same(a, b string) bool { return strings.TrimRight(a, " ") == strings.TrimRight(b, " ") }

// item is line n (1-based), which must hold text. When guard is set it must also be the text the caller saw.
func (q Queue) item(id state.ID, lines []string, n int, guard *string) (string, error) {
	if n < 1 || n > len(lines) || lines[n-1] == "" {
		return "", &NoItemError{ID: id, N: n}
	}
	if guard != nil && !same(lines[n-1], *guard) {
		return "", &ChangedError{ID: id}
	}
	return lines[n-1], nil
}

// Add appends an item and returns how many items the queue now holds.
func (q Queue) Add(id state.ID, text textsafe.Text) (int, error) {
	if strings.TrimSpace(text.String()) == "" {
		return 0, ErrBlank
	}
	if err := q.Store.Ensure(); err != nil {
		return 0, err
	}
	if err := q.append(id, text.String()); err != nil {
		return 0, err
	}
	return q.Count(id), nil
}

func (q Queue) append(id state.ID, item string) error {
	f, err := os.OpenFile(q.file(id), os.O_WRONLY|os.O_APPEND|os.O_CREATE, state.FileMode)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(item + "\n"); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Remove takes item n out and returns it.
func (q Queue) Remove(id state.ID, n int, guard *string) (textsafe.Text, error) {
	lines, err := q.lines(id)
	if err != nil {
		return textsafe.Text{}, err
	}
	if len(lines) == 0 {
		return textsafe.Text{}, ErrEmpty
	}
	it, err := q.item(id, lines, n, guard)
	if err != nil {
		return textsafe.Text{}, err
	}
	if err := q.write(id, append(lines[:n-1:n-1], lines[n:]...)); err != nil {
		return textsafe.Text{}, err
	}
	return textsafe.Strip(it), nil
}

// Reorder moves item from to position to, and returns the item.
func (q Queue) Reorder(id state.ID, from, to int, guard *string) (textsafe.Text, error) {
	lines, err := q.lines(id)
	if err != nil {
		return textsafe.Text{}, err
	}
	count := q.Count(id)
	if from < 1 || to < 1 || from > count || to > count {
		return textsafe.Text{}, &RangeError{ID: id, Count: count}
	}
	it, err := q.item(id, lines, from, guard)
	if err != nil {
		return textsafe.Text{}, err
	}
	rest := append(lines[:from-1:from-1], lines[from:]...)
	out := append(rest[:to-1:to-1], append([]string{it}, rest[to-1:]...)...)
	if err := q.write(id, out); err != nil {
		return textsafe.Text{}, err
	}
	return textsafe.Strip(it), nil
}

// Move hands item n from one queue to the end of another and returns it. The item reaches the destination before it leaves the source, so
// a failure can leave a duplicate but never lose it.
func (q Queue) Move(from, to state.ID, n int, guard *string) (textsafe.Text, error) {
	lines, err := q.lines(from)
	if err != nil {
		return textsafe.Text{}, err
	}
	it, err := q.item(from, lines, n, guard)
	if err != nil {
		return textsafe.Text{}, err
	}
	if err := q.Store.Ensure(); err != nil {
		return textsafe.Text{}, err
	}
	if err := q.append(to, it); err != nil {
		return textsafe.Text{}, fmt.Errorf("could not queue on %s", Label(to))
	}
	if err := q.write(from, append(lines[:n-1:n-1], lines[n:]...)); err != nil {
		return textsafe.Text{}, fmt.Errorf("could not update %s", Label(from))
	}
	return textsafe.Strip(it), nil
}
