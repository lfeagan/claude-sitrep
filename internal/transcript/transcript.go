// Package transcript reads Claude Code's session transcripts (JSON lines). A transcript is untrusted input: it is read only when it is a
// regular file under the projects folder, never through a link, and never more than a bounded amount.
package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
)

const (
	maxLine = 16 << 20 // the longest line read; a longer one is skipped
	maxScan = 16 << 20 // the most read looking for the first prompt, so a prompt never waits on a huge transcript
	tailMax = 8 << 20  // how far back from the end the recent lines are looked for
)

// Valid says whether path may be read: it starts with the literal <configDir>/projects/, has no /../ in it (checked on the raw string,
// before any cleaning), and is a regular file that is not a link.
func Valid(path, configDir string) bool {
	if !strings.HasPrefix(path, configDir+"/projects/") || strings.Contains(path, "/../") {
		return false
	}
	fi, err := os.Lstat(path)
	return err == nil && fi.Mode().IsRegular()
}

// open opens a transcript without following a link, so a path swapped for a link after Valid is refused.
func open(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		f.Close()
		return nil, os.ErrInvalid
	}
	return f, nil
}

type line struct {
	Type    string `json:"type"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// text is the text of a message: a string as it is, or the text parts of a list joined by a space.
func (l line) text() string {
	var s string
	if json.Unmarshal(l.Message.Content, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(l.Message.Content, &parts) != nil {
		return ""
	}
	var texts []string
	for _, p := range parts {
		if p.Type == "text" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, " ")
}

// FirstPrompt is the first line of the first real user message: messages that start with a tag (command and system wrappers) or are blank
// are skipped. A line that is not JSON is skipped.
func FirstPrompt(path string) string {
	f, err := open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	r := bufio.NewReaderSize(io.LimitReader(f, maxScan), 256<<10)
	for {
		raw, tooLong, err := readLine(r)
		if len(raw) > 0 && !tooLong {
			var l line
			if json.Unmarshal(raw, &l) == nil && l.Type == "user" {
				t := l.text()
				head := strings.TrimLeftFunc(t, unicode.IsSpace)
				if !strings.HasPrefix(head, "<") && !strings.HasPrefix(head, "&lt;") {
					for _, ln := range strings.Split(t, "\n") {
						if ln = strings.TrimLeftFunc(ln, unicode.IsSpace); ln != "" {
							return ln
						}
					}
				}
			}
		}
		if err != nil {
			return ""
		}
	}
}

// readLine reads one line. A line longer than maxLine is skipped to its end and reported as too long, so one huge entry (an image, a tool
// result) cannot end the scan.
func readLine(r *bufio.Reader) (line []byte, tooLong bool, err error) {
	for {
		part, isPrefix, err := r.ReadLine()
		if err != nil {
			return line, tooLong, err
		}
		if !tooLong {
			if len(line)+len(part) > maxLine {
				tooLong, line = true, nil
			} else {
				line = append(line, part...)
			}
		}
		if !isPrefix {
			return line, tooLong, nil
		}
	}
}

// Recent is the end of the conversation for the summarizer: the user and assistant text of the last 40 lines, "type: text" each, every
// physical line cut to 600 characters, and the last 6000 bytes of that, on a character boundary.
func Recent(path string) (string, bool) {
	f, err := open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return "", false
	}
	start := max(0, fi.Size()-tailMax)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", false
	}
	b, _ := io.ReadAll(io.LimitReader(f, tailMax))
	if start > 0 { // the first line is cut in two
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > 40 {
		lines = lines[len(lines)-40:]
	}
	var out []string
	for _, raw := range lines {
		var l line
		if json.Unmarshal([]byte(raw), &l) != nil || (l.Type != "user" && l.Type != "assistant") {
			continue
		}
		for _, ln := range strings.Split(l.Type+": "+l.text(), "\n") {
			out = append(out, cutRunes(ln, 600))
		}
	}
	s := strings.Join(out, "\n")
	if len(s) > 6000 {
		s = s[len(s)-6000:]
		for len(s) > 0 && !utf8.RuneStart(s[0]) {
			s = s[1:]
		}
	}
	return strings.TrimRight(s, "\n"), true
}

func cutRunes(s string, n int) string {
	i := 0
	for j := range s {
		if i == n {
			return s[:j]
		}
		i++
	}
	return s
}
