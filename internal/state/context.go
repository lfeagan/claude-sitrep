package state

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

// Limits of a context record. The input comes from Claude Code, and the file can be edited, so every number is range-checked.
const (
	MaxWindow     = 100_000_000
	MaxTokens     = 10_000_000_000
	ContextMaxAge = 300 // seconds after which a record is written again though its numbers are the same, so its time means last seen
)

// Context is how full a session's context window was when its status line was last drawn.
type Context struct {
	UsedPercent float64 // 0 to 100
	Window      int64   // the size of the window, in tokens
	Tokens      int64   // what was in it
	At          int64   // when the status line last showed these numbers, in epoch seconds
}

// Valid says every number is in range.
func (c Context) Valid() bool {
	return !math.IsNaN(c.UsedPercent) && c.UsedPercent >= 0 && c.UsedPercent <= 100 && c.Window >= 1 && c.Window <= MaxWindow &&
		c.Tokens >= 0 && c.Tokens <= MaxTokens && c.At >= 0
}

func (c Context) line() string {
	return fmt.Sprintf("%.1f %d %d %d\n", c.UsedPercent, c.Window, c.Tokens, c.At)
}

// sameNumbers compares what the file would hold, apart from the time.
func (c Context) sameNumbers(o Context) bool {
	return fmt.Sprintf("%.1f", c.UsedPercent) == fmt.Sprintf("%.1f", o.UsedPercent) && c.Window == o.Window && c.Tokens == o.Tokens
}

// Context is the record of a session, and whether there is a valid one. The file is the user's to edit, so only a regular file that is not
// a link, holds at most 128 bytes and has four numbers in range counts.
func (s Store) Context(id ID) (Context, bool) {
	if id == Inbox {
		return Context{}, false
	}
	path := s.Path(id, "context")
	if fi, err := os.Lstat(path); err != nil || !fi.Mode().IsRegular() {
		return Context{}, false
	}
	b, err := ReadFile(path, 129)
	if err != nil || len(b) > 128 {
		return Context{}, false
	}
	f := strings.Fields(string(b))
	if len(f) != 4 {
		return Context{}, false
	}
	var c Context
	var e1, e2, e3, e4 error
	c.UsedPercent, e1 = strconv.ParseFloat(f[0], 64)
	c.Window, e2 = strconv.ParseInt(f[1], 10, 64)
	c.Tokens, e3 = strconv.ParseInt(f[2], 10, 64)
	c.At, e4 = strconv.ParseInt(f[3], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || !c.Valid() {
		return Context{}, false
	}
	return c, true
}

// RecordContext writes the record of a session, if the session exists (its notes file does: an unknown id makes no folder) and the numbers
// are valid. It writes only when the numbers changed or the record is more than ContextMaxAge old.
func (s Store) RecordContext(id ID, c Context) error {
	if id == Inbox || !c.Valid() {
		return nil
	}
	if !IsRegular(s.Path(id, "notes")) {
		return nil
	}
	if old, ok := s.Context(id); ok && old.sameNumbers(c) && c.At-old.At >= 0 && c.At-old.At < ContextMaxAge {
		return nil
	}
	return s.WriteFile(s.Path(id, "context"), []byte(c.line()))
}
