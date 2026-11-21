// Package conf reads sitrep's settings: the environment first, then sitrep.conf in the state folder. The file is read, never run, so it
// holds NAME=value lines only (single quotes, double quotes and backslashes are understood; comments, export, variables and commands
// are not). The bash status line used to source the file, which let it override the environment and run code; that is gone.
package conf

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Defaults of the settings that have one.
const (
	DefaultQueueItems       = 5
	DefaultHighlightSeconds = 60
	DefaultHighlightSGR     = "48;5;238"
)

// Conf looks settings up.
type Conf struct {
	File   string              // path of sitrep.conf
	Getenv func(string) string // the environment; os.Getenv when nil
}

// Open names the conf file in a state folder.
func Open(stateDir string) Conf { return Conf{File: filepath.Join(stateDir, "sitrep.conf")} }

func (c Conf) env(name string) string {
	if c.Getenv != nil {
		return c.Getenv(name)
	}
	return os.Getenv(name)
}

// Get returns a setting: the environment wins, then the first NAME=value line of the file. It is empty when unset.
func (c Conf) Get(name string) string {
	if v := c.env(name); v != "" {
		return v
	}
	b := readSmall(c.File)
	prefix := name + "="
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(line, prefix); ok {
			return Unquote(rest)
		}
	}
	return ""
}

// QueueItems is how many queue items a listing shows (SITREP_QUEUE_ITEMS).
func (c Conf) QueueItems() int { return c.number("SITREP_QUEUE_ITEMS", DefaultQueueItems) }

// HighlightSeconds is how long a session counts as recently active (SITREP_HIGHLIGHT_SECONDS); 0 turns the highlight off.
func (c Conf) HighlightSeconds() int {
	return c.number("SITREP_HIGHLIGHT_SECONDS", DefaultHighlightSeconds)
}

// HighlightSGR is the terminal style of the highlight. Only digits and semicolons are accepted, because the value reaches the terminal.
func (c Conf) HighlightSGR() string {
	v := c.Get("SITREP_HIGHLIGHT_SGR")
	if v == "" {
		return DefaultHighlightSGR
	}
	for _, r := range v {
		if !(r >= '0' && r <= '9' || r == ';') {
			return DefaultHighlightSGR
		}
	}
	return v
}

func (c Conf) number(name string, def int) int {
	v := c.Get(name)
	for _, r := range v {
		if r < '0' || r > '9' {
			return def
		}
	}
	n, err := strconv.Atoi(v)
	if v == "" || err != nil {
		return def
	}
	return n
}

// Unquote reads one shell word the way sitrep.conf holds it, without running anything: single quotes, double quotes and backslash escapes
// are understood, and variables and commands are left as they are.
func Unquote(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\'':
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				b.WriteString(s[i+1:])
				return b.String()
			}
			b.WriteString(s[i+1 : i+1+j])
			i += j + 1
		case c == '"':
			for i++; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' && i+1 < len(s) && strings.IndexByte("\"\\$`", s[i+1]) >= 0 {
					i++
				}
				b.WriteByte(s[i])
			}
		case c == '\\' && i+1 < len(s):
			i++
			b.WriteByte(s[i])
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// readSmall reads a regular file of at most 64 KB. Anything else (a pipe, a huge file, a missing one) reads as empty, so a hook never hangs.
func readSmall(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() || fi.Size() > 64<<10 {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(f, 64<<10))
	return b
}
