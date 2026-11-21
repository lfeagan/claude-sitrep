// Package conf reads sitrep's settings: the environment first, then sitrep.conf in the state folder. The file is read, never run, so it
// holds NAME=value lines only (single quotes, double quotes, backslashes and $'...' are understood, so what printf %q wrote reads back; comments, export, variables
// and commands are not). The bash status line used to source the file, which let it override the environment and run code; that is gone.
package conf

import (
	"errors"
	"github.com/lfeagan/claude-sitrep/internal/state"
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
	return c.FromFile(name)
}

// FromFile is a setting as the file holds it, whatever the environment says: setup and its removal save and restore what is in the file.
func (c Conf) FromFile(name string) string {
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

// ansiC reads the inside of $'...' that starts at s[i], writes it to b, and returns the index of the closing quote. It knows the escapes
// that printf %q writes: \n \t \r \a \b \e \f \v, \\ \' \" \?, \xHH, \NNN (octal) and \uHHHH or \UHHHHHHHH.
func ansiC(b *strings.Builder, s string, i int) int {
	for ; i < len(s) && s[i] != '\''; i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch c := s[i]; c {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case 'a':
			b.WriteByte(7)
		case 'b':
			b.WriteByte(8)
		case 'e', 'E':
			b.WriteByte(27)
		case 'f':
			b.WriteByte(12)
		case 'v':
			b.WriteByte(11)
		case 'x', 'u', 'U':
			n := map[byte]int{'x': 2, 'u': 4, 'U': 8}[c]
			j, v := i+1, 0
			for ; j < len(s) && j < i+1+n && isHex(s[j]); j++ {
				v = v*16 + hexVal(s[j])
			}
			if j == i+1 {
				b.WriteByte(c)
			} else if c == 'x' {
				b.WriteByte(byte(v))
				i = j - 1
			} else {
				b.WriteRune(rune(v))
				i = j - 1
			}
		case '0', '1', '2', '3', '4', '5', '6', '7':
			j, v := i, 0
			for ; j < len(s) && j < i+3 && s[j] >= '0' && s[j] <= '7'; j++ {
				v = v*8 + int(s[j]-'0')
			}
			b.WriteByte(byte(v))
			i = j - 1
		default:
			b.WriteByte(c)
		}
	}
	return i
}

func isHex(c byte) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }

func hexVal(c byte) int {
	switch {
	case c >= 'a':
		return int(c-'a') + 10
	case c >= 'A':
		return int(c-'A') + 10
	}
	return int(c - '0')
}

// Unquote reads one shell word the way sitrep.conf holds it, without running anything: single quotes, double quotes and backslash escapes
// and $'...' are understood, and variables and commands are left as they are.
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
		case c == '$' && i+1 < len(s) && s[i+1] == '\'':
			i = ansiC(&b, s, i+2)
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
	b, err := state.ReadFile(path, 64<<10+1)
	if err != nil || len(b) > 64<<10 {
		return nil
	}
	return b
}

// hudKey names the line that holds the upstream status line.
const hudKey = "SITREP_HUD_CMD"

// ErrMultiline says a command cannot be kept in sitrep.conf, which holds one setting per line.
var ErrMultiline = errors.New("the command has more than one line")

// Quote writes s as one shell word that Unquote reads back: in single quotes, a single quote written as '\”.
func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (c Conf) store() state.Store { return state.Store{Dir: filepath.Dir(c.File)} }

// DropHUD removes our SITREP_HUD_CMD line. Every other line is the user's and stays; the file goes when nothing else is left.
func (c Conf) DropHUD() error {
	b, err := state.ReadFile(c.File, 64<<10)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var keep []string
	for _, l := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(l, hudKey+"=") {
			keep = append(keep, l)
		}
	}
	rest := strings.Join(keep, "\n")
	if strings.TrimSpace(strings.ReplaceAll(rest, "\n", "")) == "" {
		return os.Remove(c.File)
	}
	return c.store().WriteFile(c.File, []byte(rest))
}

// PutHUD saves the upstream status line, replacing any earlier one and keeping the user's own lines.
func (c Conf) PutHUD(cmd string) error {
	if strings.ContainsAny(cmd, "\n\r") {
		return ErrMultiline
	}
	if err := c.DropHUD(); err != nil {
		return err
	}
	b, err := state.ReadFile(c.File, 64<<10)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	body := string(b)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return c.store().WriteFile(c.File, []byte(body+hudKey+"="+Quote(cmd)+"\n"))
}
