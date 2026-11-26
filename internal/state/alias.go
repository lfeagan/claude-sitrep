package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Longest alias, in characters.
const MaxAlias = 32

// ValidAlias says s may be an alias: lowercase letters, digits and hyphens, starting with a letter, at most MaxAlias characters. The inbox
// is reserved. The grammar is what makes an alias safe to print and impossible to mistake for a flag or an item number.
func ValidAlias(s string) bool {
	if s == string(Inbox) || len(s) == 0 || len(s) > MaxAlias || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// Alias is the name the user gave a session, or "" when there is none. The file is the user's to edit, so only a regular file that is not a
// link, holds at most 64 bytes and has a first line that passes ValidAlias counts; anything else is no alias.
func (s Store) Alias(id ID) string {
	if id == Inbox {
		return ""
	}
	path := s.Path(id, "alias")
	if fi, err := os.Lstat(path); err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	b, err := ReadFile(path, 65)
	if err != nil || len(b) > 64 {
		return ""
	}
	if name, _, _ := strings.Cut(string(b), "\n"); ValidAlias(name) {
		return name
	}
	return ""
}

// Aliases maps each session that has an alias to it.
func (s Store) Aliases() map[ID]string {
	out := map[ID]string{}
	for _, id := range s.Sessions() {
		if a := s.Alias(id); a != "" {
			out[id] = a
		}
	}
	return out
}

// short is the first four characters of an id, which is what a person is shown of a session that has no alias.
func short(id ID) string {
	if len(id) > 4 {
		return string(id[:4])
	}
	return string(id)
}

// AliasTakenError says another session has the alias.
type AliasTakenError struct {
	Name   string
	Holder ID
}

func (e *AliasTakenError) Error() string {
	return fmt.Sprintf("'%s' is already the alias of %s; free it with 'sitrep alias %s --off'", e.Name, short(e.Holder), short(e.Holder))
}

// AliasPrefixError says the name is the start of a session id, which `Resolve` reads as that id.
type AliasPrefixError struct {
	Name string
	ID   ID
}

func (e *AliasPrefixError) Error() string {
	return fmt.Sprintf("'%s' is the start of the id of %s, so it would name that session; choose a name with a letter after f", e.Name, short(e.ID))
}

// SetAlias gives a session an alias and says whether anything changed. A name that another session holds is refused, whatever that session's
// state, as an ended session may be resumed. The check and the write happen under the folder's lock.
func (s Store) SetAlias(id ID, name string) (bool, error) {
	changed, _, err := s.SetAliasFrom(id, name, nil)
	return changed, err
}

// SetAliasFrom is SetAlias for a caller that may take the name from the session that holds it: mayTake says, for that holder, whether it
// may (the caller asked for it, and the holder has ended). The holder loses the name in the same locked step as the new session gets it,
// and `from` is that holder, or "" when the name was free.
func (s Store) SetAliasFrom(id ID, name string, mayTake func(holder ID) bool) (changed bool, from ID, err error) {
	if id == Inbox {
		return false, "", fmt.Errorf("the inbox has no alias")
	}
	if !ValidAlias(name) {
		return false, "", fmt.Errorf("an alias is 1 to %d lowercase letters, digits and hyphens, starting with a letter, and not 'inbox'", MaxAlias)
	}
	unlock, err := s.Lock()
	if err != nil {
		return false, "", err
	}
	defer unlock()
	for _, o := range s.Sessions() {
		if o != id && s.Alias(o) == name {
			if mayTake == nil || !mayTake(o) {
				return false, "", &AliasTakenError{Name: name, Holder: o}
			}
			from = o
		}
		if strings.HasPrefix(string(o), name) {
			return false, "", &AliasPrefixError{Name: name, ID: o}
		}
	}
	if from != "" {
		if err := os.Remove(s.Path(from, "alias")); err != nil && !os.IsNotExist(err) {
			return false, "", err
		}
	}
	if s.Alias(id) == name {
		return false, from, nil
	}
	return true, from, s.WriteFile(s.Path(id, "alias"), []byte(name+"\n"))
}

// ClearAlias removes a session's alias and returns the name it had ("" when there was none).
func (s Store) ClearAlias(id ID) (string, error) {
	if id == Inbox {
		return "", fmt.Errorf("the inbox has no alias")
	}
	unlock, err := s.Lock()
	if err != nil {
		return "", err
	}
	defer unlock()
	old := s.Alias(id)
	if err := os.Remove(s.Path(id, "alias")); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return old, nil
}

// AliasEntry is one alias and the session that has it.
type AliasEntry struct {
	Name string
	ID   ID
}

// AliasList is the aliases, sorted by name.
func (s Store) AliasList() []AliasEntry {
	var out []AliasEntry
	for id, a := range s.Aliases() {
		out = append(out, AliasEntry{a, id})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Lock takes the lock every change to a queue or an alias takes, so that one lock covers the whole folder. Nothing to protect when the
// folder does not exist yet.
func (s Store) Lock() (func(), error) {
	if _, err := os.Stat(s.Dir); os.IsNotExist(err) {
		return func() {}, nil
	}
	return Lock(filepath.Join(s.Dir, ".queue.lock"))
}
