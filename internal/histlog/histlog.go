// Package histlog is the history log: one JSON line appended for each change sitrep makes to its state, and none for a read. It is a record
// beside the files, which stay the source of truth, and nothing replays it. See docs/HISTORY-LOG.md.
package histlog

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// The operations a log records.
const (
	QueueAdd     = "queue.add"
	QueueRm      = "queue.rm"
	QueueEdit    = "queue.edit"
	QueueMove    = "queue.move"
	QueueReorder = "queue.reorder"
	Pin          = "pin"
	Unpin        = "unpin"
	Snooze       = "snooze"
	Unsnooze     = "unsnooze"
	Alias        = "alias"
	Prune        = "prune"
	SessionStart = "session.start"
	SessionEnd   = "session.end"
)

var ops = map[string]bool{QueueAdd: true, QueueRm: true, QueueEdit: true, QueueMove: true, QueueReorder: true, Pin: true, Unpin: true, Snooze: true, Unsnooze: true, Alias: true, Prune: true, SessionStart: true, SessionEnd: true}

// KnownOp says whether op is one of the operations a log records.
func KnownOp(op string) bool { return ops[op] }

// maxLine bounds a line, so a single write stays small. An entry that cannot fit is dropped.
const maxLine = 4096

// maxFile bounds what a reader takes from one file.
const maxFile = 64 << 20

// Entry is one change. It describes the outcome, not the command: items are named by their text, and a position is a fact about the moment.
type Entry struct {
	Op   string
	ID   state.ID      // the session, or the inbox
	Text textsafe.Text // the item, the Objective or the name
	Old  textsafe.Text // queue.edit: the text before; alias: the name before
	From int           // queue.reorder: the position the item left
	To   int           // queue.reorder: the position it took
	Dest state.ID      // queue.move: the queue it went to, with ID as the queue it left
}

// Log writes entries to one host's files. The zero Log is off and writes nothing.
type Log struct {
	Dir   string // the state folder
	Host  string // the name of this host, as Host makes it
	On    bool
	MaxMB int // the size limit for both files together
	Days  int // the age limit
	Now   func() time.Time
}

// Host makes a name for a machine that is safe as a folder name: letters, digits, underscore and hyphen, at most 64 of them.
func Host(name string) string {
	var b strings.Builder
	for _, r := range name {
		if b.Len() >= 64 {
			break
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else if r != '.' || b.Len() > 0 {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 || b.String() == "inbox" {
		return "host"
	}
	return b.String()
}

func (l Log) hostDir() string { return filepath.Join(l.Dir, "log", l.Host) }
func (l Log) current() string { return filepath.Join(l.hostDir(), "current.jsonl") }
func (l Log) previous() string {
	return filepath.Join(l.hostDir(), "previous.jsonl")
}
func (l Log) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

type line struct {
	E    string `json:"e"`
	T    string `json:"t"`
	Host string `json:"host"`
	ID   string `json:"id"`
	Op   string `json:"op"`
	Text string `json:"text,omitempty"`
	Old  string `json:"old,omitempty"`
	From int    `json:"from,omitempty"`
	To   int    `json:"to,omitempty"`
	Dest string `json:"dest,omitempty"`
}

// Record appends an entry. It is called after the change succeeded, and it never fails: on any error the entry is dropped (invariant 4).
func (l Log) Record(e Entry) {
	if !l.On || l.Host == "" || l.Dir == "" || !ops[e.Op] {
		return
	}
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return
	}
	ln := line{E: hex.EncodeToString(raw[:]), T: l.now().UTC().Format(time.RFC3339), Host: l.Host, ID: string(e.ID), Op: e.Op,
		Text: textsafe.Truncate(e.Text, 200).String(), Old: textsafe.Truncate(e.Old, 200).String(), From: e.From, To: e.To, Dest: string(e.Dest)}
	b := encode(ln)
	if len(b) > maxLine {
		ln.Old = ""
		b = encode(ln)
	}
	if len(b) > maxLine {
		return
	}
	if err := l.ensure(); err != nil {
		return
	}
	l.rollIfDue(int64(len(b)))
	f, err := os.OpenFile(l.current(), os.O_WRONLY|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, state.FileMode)
	if err != nil {
		return
	}
	_, _ = f.Write(b)
	_ = f.Close()
}

func encode(ln line) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(ln) != nil {
		return nil
	}
	return buf.Bytes()
}

// ensure makes the folders, owner-only, and refuses a link or a file where a folder belongs. Each level is checked before the next is made, so
// nothing is made through a link.
func (l Log) ensure() error {
	if err := os.MkdirAll(l.Dir, state.DirMode); err != nil {
		return err
	}
	for _, d := range []string{filepath.Join(l.Dir, "log"), l.hostDir()} {
		if err := os.Mkdir(d, state.DirMode); err != nil && !os.IsExist(err) {
			return err
		}
		if fi, err := os.Lstat(d); err != nil || !fi.IsDir() {
			return os.ErrInvalid
		}
	}
	return nil
}

// rollIfDue renames current over previous when current has reached half of a limit: half of the size, or half of the age. It takes the lock
// for one try only, so a hook never waits, and looks again once it holds it. When the lock is busy the entry is appended anyway and a later
// call rolls.
func (l Log) rollIfDue(next int64) {
	if !l.due(next) {
		return
	}
	unlock, ok := state.TryLock(filepath.Join(l.hostDir(), "lock"))
	if !ok {
		return
	}
	defer unlock()
	if l.due(next) {
		_ = os.Rename(l.current(), l.previous())
	}
}

func (l Log) due(next int64) bool {
	fi, err := os.Lstat(l.current())
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	maxMB, days := l.MaxMB, l.Days
	if maxMB < 1 {
		maxMB = 10
	}
	if days < 1 {
		days = 30
	}
	if fi.Size()+next > int64(maxMB)<<20/2 {
		return true
	}
	first, ok := l.firstTime()
	return ok && l.now().Sub(first) >= time.Duration(days)*24*time.Hour/2
}

// firstTime is the time of the first entry of current.
func (l Log) firstTime() (time.Time, bool) {
	b, err := state.ReadFile(l.current(), maxLine)
	if err != nil {
		return time.Time{}, false
	}
	first, _, _ := bytes.Cut(b, []byte("\n"))
	var ln struct {
		T string `json:"t"`
	}
	if json.Unmarshal(first, &ln) != nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, ln.T)
	return t, err == nil
}

// Item is an entry as a reader sees it. Every text in it has been made safe again, because the file is untrusted.
type Item struct {
	E    string
	Time time.Time
	Host string
	ID   state.ID
	Op   string
	Text textsafe.Text
	Old  textsafe.Text
	From int
	To   int
	Dest state.ID
}

// Query chooses what Read returns.
type Query struct {
	Host  string   // only this host; empty means every host in the folder
	ID    state.ID // only this queue or session (as a source or a destination); empty means all
	Op    string   // only this operation
	Since time.Time
	Days  int // entries older than this many days are ignored, whatever Since says
	Now   time.Time
	Limit int // the most recent this many, or all when below 1
}

// Read returns the entries that match, oldest first. A line that does not parse is skipped, which covers a torn last line, and a line whose
// entry id was already seen is a repeat and is dropped. It reads a host's previous file and then its current one.
func Read(dir string, q Query) []Item {
	hosts := []string{q.Host}
	if q.Host == "" {
		hosts = nil
		entries, _ := os.ReadDir(filepath.Join(dir, "log"))
		for _, e := range entries {
			if e.IsDir() && Host(e.Name()) == e.Name() {
				hosts = append(hosts, e.Name())
			}
		}
	} else if Host(q.Host) != q.Host {
		return nil
	}
	cutoff := q.Since
	if q.Days > 0 {
		if c := q.Now.Add(-time.Duration(q.Days) * 24 * time.Hour); c.After(cutoff) {
			cutoff = c
		}
	}
	seen := map[string]bool{}
	var out []Item
	for _, h := range hosts {
		for _, name := range []string{"previous.jsonl", "current.jsonl"} {
			b, err := state.ReadFile(filepath.Join(dir, "log", h, name), maxFile)
			if err != nil {
				continue
			}
			sc := bufio.NewScanner(bytes.NewReader(b))
			sc.Buffer(make([]byte, 0, 64<<10), maxLine*4)
			for sc.Scan() {
				it, ok := parse(sc.Bytes())
				if !ok || it.Host != h || seen[it.E] {
					continue
				}
				seen[it.E] = true
				if it.Time.Before(cutoff) || q.Op != "" && it.Op != q.Op || q.ID != "" && it.ID != q.ID && it.Dest != q.ID {
					continue
				}
				out = append(out, it)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time.Before(out[j].Time) })
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[len(out)-q.Limit:]
	}
	return out
}

func parse(b []byte) (Item, bool) {
	var ln line
	if json.Unmarshal(b, &ln) != nil {
		return Item{}, false
	}
	t, err := time.Parse(time.RFC3339, ln.T)
	id, idErr := state.ParseID(ln.ID)
	if err != nil || idErr != nil || !ops[ln.Op] || ln.E == "" || len(ln.E) > 32 || Host(ln.Host) != ln.Host {
		return Item{}, false
	}
	var dest state.ID
	if ln.Dest != "" {
		if dest, err = state.ParseID(ln.Dest); err != nil {
			return Item{}, false
		}
	}
	return Item{E: textsafe.Clean(ln.E, 32).String(), Time: t, Host: ln.Host, ID: id, Op: ln.Op, Text: textsafe.Clean(ln.Text, 200), Old: textsafe.Clean(ln.Old, 200),
		From: ln.From, To: ln.To, Dest: dest}, true
}

// Clear removes this host's files, each by its own path, and then its folder if that is now empty.
func (l Log) Clear() error {
	var first error
	for _, p := range []string{l.current(), l.previous(), filepath.Join(l.hostDir(), "lock")} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
	}
	_ = os.Remove(l.hostDir())
	_ = os.Remove(filepath.Join(l.Dir, "log"))
	return first
}
