// Package settings edits Claude Code's settings.json in place: only the keys sitrep owns change, and the rest of the file (other keys, their
// order, the user's formatting) stays byte for byte. An edit is checked before anything is written, written to a temporary file, and moved
// into place, so Claude Code never sees half a file and a symlinked settings.json stays a symlink.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/lfeagan/claude-sitrep/internal/state"
)

// Errors a caller reports as they are.
var (
	ErrNotJSON       = errors.New("cannot be parsed as JSON")
	ErrNotObject     = errors.New("is not a JSON object")
	ErrStatusNotObj  = errors.New("the statusLine is not an object")
	ErrStatusMissing = errors.New("there is no statusLine")
)

// Doc is the text of a settings file.
type Doc []byte

// Parse checks that data is one JSON object. The edit library accepts malformed input and a byte order mark silently drops the rest of the
// file, so nothing is edited until this passes.
func Parse(data []byte) (Doc, error) {
	if !json.Valid(data) {
		return nil, ErrNotJSON
	}
	if !gjson.ParseBytes(data).IsObject() {
		return nil, ErrNotObject
	}
	// jq and Node keep the last of two equal keys, and the edit library acts on the first, so an edit could silently not take effect
	if dup, err := duplicateKey(data); err != nil {
		return nil, ErrNotJSON
	} else if dup != "" {
		return nil, fmt.Errorf("%w: %q appears twice", ErrDuplicate, dup)
	}
	return Doc(data), nil
}

// ErrDuplicate says an object has the same key twice.
var ErrDuplicate = errors.New("has a duplicate key")

// BOM is the byte order mark some editors put at the start of a file. It is kept as it is, outside the document.
const BOM = "\xef\xbb\xbf"

// SplitBOM takes a leading byte order mark off, so that the rest can be parsed and edited.
func SplitBOM(b []byte) (bom string, rest []byte) {
	if strings.HasPrefix(string(b), BOM) {
		return BOM, b[len(BOM):]
	}
	return "", b
}

// duplicateKey finds a key that occurs twice in the top-level object or in its statusLine object, and returns it.
func duplicateKey(data []byte) (string, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	if _, err := dec.Token(); err != nil { // the opening brace
		return "", err
	}
	seen := map[string]bool{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return "", err
		}
		key, _ := kt.(string)
		if seen[key] {
			return key, nil
		}
		seen[key] = true
		if key != "statusLine" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return "", err
			}
			continue
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return "", err
		}
		if d, err := keysOf(raw); err != nil {
			return "", err
		} else if d != "" {
			return "statusLine." + d, nil
		}
	}
	return "", nil
}

// keysOf returns a duplicate key of an object, if raw is one.
func keysOf(raw json.RawMessage) (string, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	t, err := dec.Token()
	if err != nil {
		return "", err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return "", nil
	}
	seen := map[string]bool{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return "", err
		}
		key, _ := kt.(string)
		if seen[key] {
			return key, nil
		}
		seen[key] = true
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return "", err
		}
	}
	return "", nil
}

// New is an empty settings file.
func New() Doc { return Doc("{}\n") }

// StatusLine is the status line command, and whether the key holds an object (or nothing, or null), which is the only shape setup edits.
func (d Doc) StatusLine() (command string, editable bool) {
	r := gjson.GetBytes(d, "statusLine")
	switch {
	case !r.Exists() || r.Type == gjson.Null:
		return "", true
	case r.IsObject():
		c := gjson.GetBytes(d, "statusLine.command")
		if c.Exists() && c.Type != gjson.String {
			return "", false
		}
		return c.String(), true
	}
	return "", false
}

// SetStatusLine makes the status line a command. Other keys of the statusLine object, such as padding, stay.
func (d Doc) SetStatusLine(command string) (Doc, error) {
	if _, ok := d.StatusLine(); !ok {
		return nil, ErrStatusNotObj
	}
	out, err := sjson.SetBytes(d, "statusLine.type", "command")
	if err != nil {
		return nil, err
	}
	out, err = sjson.SetBytes(out, "statusLine.command", command)
	return Doc(out), err
}

// Verify checks an edit before it is written: the result is valid, every other top-level key holds what it held, and the status line is what
// was meant (command empty means it was removed). The edit library does not check its own work.
func Verify(before, after Doc, command string) error {
	if _, err := Parse(after); err != nil {
		return fmt.Errorf("the edited file is not valid: %w", err)
	}
	old, new := topLevel(before), topLevel(after)
	for k, v := range old {
		if k == "statusLine" {
			continue
		}
		if nv, ok := new[k]; !ok || nv != v {
			return fmt.Errorf("the edit would change %q", k)
		}
	}
	for k := range new {
		if _, ok := old[k]; !ok && k != "statusLine" {
			return fmt.Errorf("the edit would add %q", k)
		}
	}
	got := gjson.GetBytes(after, "statusLine.command")
	switch {
	case command == "" && gjson.GetBytes(after, "statusLine").Exists():
		return errors.New("the status line was not removed")
	case command != "" && (got.String() != command || !got.Exists()):
		return errors.New("the status line command is not what was meant")
	}
	return nil
}

// topLevel maps each top-level key to its value in compact form.
func topLevel(d Doc) map[string]string {
	m := map[string]string{}
	gjson.ParseBytes(d).ForEach(func(k, v gjson.Result) bool {
		m[k.String()] = compact(v.Raw)
		return true
	})
	return m
}

func compact(raw string) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(raw)); err != nil {
		return raw
	}
	return buf.String()
}

// RestoreStatusLine puts the upstream command back, or removes the status line when there was none.
func (d Doc) RestoreStatusLine(upstream string) (Doc, error) {
	if upstream == "" {
		out, err := sjson.DeleteBytes(d, "statusLine")
		return Doc(out), err
	}
	out, err := sjson.SetBytes(d, "statusLine.command", upstream)
	return Doc(out), err
}

// HookCommands lists the commands of every hook of every event.
func (d Doc) HookCommands() []string {
	var out []string
	gjson.GetBytes(d, "hooks").ForEach(func(_, groups gjson.Result) bool {
		groups.ForEach(func(_, group gjson.Result) bool {
			group.Get("hooks").ForEach(func(_, h gjson.Result) bool {
				if c := h.Get("command"); c.Type == gjson.String {
					out = append(out, c.String())
				}
				return true
			})
			return true
		})
		return true
	})
	return out
}

// Read reads a settings file through any link. It reports whether the file exists.
func Read(path string) (Doc, bool, error) {
	b, err := state.ReadFile(path, 16<<20)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return Doc(b), true, nil
}

// Backup writes the given bytes (what was read and parsed) to <path>.bak.<timestamp>, owner-only because settings may hold credentials. It never
// overwrites an earlier backup: a name that is taken gets a number. It returns the name.
func Backup(path string, content []byte, now time.Time) (string, error) {
	base := path + ".bak." + now.Format("20060102T150405")
	for i := 0; i < 1000; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s.%d", base, i)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, state.FileMode)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := f.Write(content); err != nil {
			f.Close()
			os.Remove(name)
			return "", err
		}
		return name, f.Close()
	}
	return "", errors.New("too many backups with the same name")
}

// ErrChanged says the file changed while setup worked on it.
var ErrChanged = errors.New("changed while it was being edited")

// resolve follows links to the file that really holds the text, even when the last one dangles (the file is then created where it points).
func resolve(path string) (string, error) {
	for i := 0; i < 16; i++ {
		fi, err := os.Lstat(path)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			return path, nil
		}
		t, err := os.Readlink(path)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(t) {
			t = filepath.Join(filepath.Dir(path), t)
		}
		path = t
	}
	return "", errors.New("too many levels of links")
}

// Write replaces the file in one step. A link is followed, so the real file is replaced and the link stays a link. The mode of an existing
// file is kept; a new one is owner-only. original is what the file held when it was read (nil for none): when it holds something else by the
// time of the rename, Claude Code or the user has written it too, and Write stops and changes nothing. That narrows the race; it cannot
// close it without a lock both sides take.
func Write(path string, d Doc, original []byte) error {
	target, err := resolve(path)
	if err != nil {
		return err
	}
	mode := state.FileMode
	if fi, err := os.Stat(target); err == nil {
		mode = fi.Mode().Perm()
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".settings.*")
	if err != nil {
		return err
	}
	name := f.Name()
	fail := func(err error) error {
		f.Close()
		os.Remove(name)
		return err
	}
	if _, err := f.Write(d); err != nil {
		return fail(err)
	}
	if err := f.Chmod(mode); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return err
	}
	now, rerr := os.ReadFile(target)
	if (rerr == nil && !bytes.Equal(now, original)) || (rerr != nil && original != nil) {
		os.Remove(name)
		return ErrChanged
	}
	if err := os.Rename(name, target); err != nil {
		os.Remove(name)
		return fmt.Errorf("could not replace %s: %w", filepath.Base(target), err)
	}
	return nil
}

// Ours reports whether a command is a sitrep status line: it ends in the tag.
func Ours(command string) bool { return strings.HasSuffix(command, Tag) }

// Tag ends the status line command sitrep writes. It is how any clone's wrapper is recognised as ours (invariant 6).
const Tag = "# claude-sitrep"
