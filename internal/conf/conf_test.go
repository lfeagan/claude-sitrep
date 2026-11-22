package conf

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestUnquote(t *testing.T) {
	for in, want := range map[string]string{
		`plain`:                   "plain",
		`'single quoted'`:         "single quoted",
		`"double \"quoted\" \$x"`: `double "quoted" $x`,
		`a\ b`:                    "a b",
		`'it'\''s'`:               "it's",
		`'a'"b"c`:                 "abc",
		`'open`:                   "open",
		`$HOME`:                   "$HOME",
		``:                        "",
	} {
		if got := Unquote(in); got != want {
			t.Errorf("Unquote(%q) = %q, want %q", in, got, want)
		}
	}
}

func conf(t *testing.T, body string, env map[string]string) Conf {
	t.Helper()
	f := filepath.Join(t.TempDir(), "sitrep.conf")
	if body != "\x00" {
		if err := os.WriteFile(f, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return Conf{File: f, Getenv: func(k string) string { return env[k] }}
}

func TestGet(t *testing.T) {
	c := conf(t, "OTHER=1\nSITREP_QUEUE_ITEMS=7\nSITREP_QUEUE_ITEMS=9\nSITREP_HUD_CMD='bash \"$HOME/x\"'\n", nil)
	if got := c.Get("SITREP_QUEUE_ITEMS"); got != "7" {
		t.Errorf("first line wins: got %q", got)
	}
	if got := c.Get("SITREP_HUD_CMD"); got != `bash "$HOME/x"` {
		t.Errorf("quoted value: got %q", got)
	}
	if got := c.Get("MISSING"); got != "" {
		t.Errorf("missing: got %q", got)
	}
	if got := c.Get("ITEMS"); got != "" {
		t.Errorf("a name is matched whole: got %q", got)
	}
	// the environment wins over the file
	c = conf(t, "SITREP_QUEUE_ITEMS=7\n", map[string]string{"SITREP_QUEUE_ITEMS": "3"})
	if got := c.QueueItems(); got != 3 {
		t.Errorf("env: got %d", got)
	}
	if got := conf(t, "\x00", nil).QueueItems(); got != DefaultQueueItems {
		t.Errorf("no file: got %d", got)
	}
}

func TestNumbers(t *testing.T) {
	for body, want := range map[string]int{
		"SITREP_QUEUE_ITEMS=0\n": 0, "SITREP_QUEUE_ITEMS=12\n": 12, "SITREP_QUEUE_ITEMS=x\n": 5, "SITREP_QUEUE_ITEMS=-1\n": 5,
		"SITREP_QUEUE_ITEMS=\n": 5, "SITREP_QUEUE_ITEMS='4'\n": 4, "SITREP_QUEUE_ITEMS=99999999999999999999\n": 5,
	} {
		if got := conf(t, body, nil).QueueItems(); got != want {
			t.Errorf("%q gives %d, want %d", body, got, want)
		}
	}
	if got := conf(t, "SITREP_HIGHLIGHT_SECONDS=0\n", nil).HighlightSeconds(); got != 0 {
		t.Errorf("highlight seconds 0 gives %d", got)
	}
	if got := conf(t, "\x00", nil).HighlightSeconds(); got != DefaultHighlightSeconds {
		t.Errorf("default highlight seconds %d", got)
	}
}

func TestHighlightSGR(t *testing.T) {
	for body, want := range map[string]string{
		"SITREP_HIGHLIGHT_SGR=1;32\n":         "1;32",
		"SITREP_HIGHLIGHT_SGR=48;5;22\n":      "48;5;22",
		"SITREP_HIGHLIGHT_SGR=1m\\033]0;x\n":  DefaultHighlightSGR,
		"SITREP_HIGHLIGHT_SGR=\n":             DefaultHighlightSGR,
		"SITREP_HIGHLIGHT_SGR='31;1'\n":       "31;1",
		"SITREP_HIGHLIGHT_SGR=$'\\x1b[31m'\n": DefaultHighlightSGR,
	} {
		if got := conf(t, body, nil).HighlightSGR(); got != want {
			t.Errorf("%q gives %q, want %q", body, got, want)
		}
	}
}

func TestOpen(t *testing.T) {
	if got := Open("/s").File; got != "/s/sitrep.conf" {
		t.Errorf("File = %q", got)
	}
}

func TestFileThatIsNotSmallAndRegularReadsAsEmpty(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.conf")
	if err := os.WriteFile(big, []byte("X=1\n"+strings.Repeat("#", 70<<10)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := (Conf{File: big, Getenv: func(string) string { return "" }}).Get("X"); got != "" {
		t.Errorf("a huge file was read: %q", got)
	}
	if got := (Conf{File: dir, Getenv: func(string) string { return "" }}).Get("X"); got != "" {
		t.Errorf("a directory was read: %q", got)
	}
}

func TestAPipeAsTheConfFileDoesNotHang(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sitrep.conf")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Skip("no mkfifo")
	}
	done := make(chan string)
	go func() { done <- (Conf{File: p, Getenv: func(string) string { return "" }}).Get("X") }()
	select {
	case got := <-done:
		if got != "" {
			t.Errorf("got %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reading the conf hung on a pipe")
	}
}

func writeConf(t *testing.T, body string) Conf {
	t.Helper()
	f := filepath.Join(t.TempDir(), "sitrep.conf")
	if body != "\x00" {
		os.WriteFile(f, []byte(body), 0o600)
	}
	return Conf{File: f, Getenv: func(string) string { return "" }}
}

func TestQuoteRoundTrips(t *testing.T) {
	for _, s := range []string{"plain", "with space", `bash "$HOME/x" # tag`, "it's", "a'b'c", `back\slash`, "$(rm -rf /)", "`x`", "", "ünï 世界"} {
		if got := Unquote(Quote(s)); got != s {
			t.Errorf("Unquote(Quote(%q)) = %q", s, got)
		}
	}
}

func TestPutAndDropHUDKeepTheUsersLines(t *testing.T) {
	c := writeConf(t, "SITREP_QUEUE_ITEMS=7\nSITREP_HUD_CMD=old\n# a comment\n")
	if err := c.PutHUD("my 'hud' $cmd"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(c.File)
	if string(b) != "SITREP_QUEUE_ITEMS=7\n# a comment\nSITREP_HUD_CMD='my '\\''hud'\\'' $cmd'\n" {
		t.Errorf("file %q", b)
	}
	if got := c.Get("SITREP_HUD_CMD"); got != "my 'hud' $cmd" {
		t.Errorf("read back %q", got)
	}
	if fi, _ := os.Stat(c.File); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	if err := c.DropHUD(); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(c.File)
	if string(b) != "SITREP_QUEUE_ITEMS=7\n# a comment\n" {
		t.Errorf("after drop %q", b)
	}
}

func TestDropHUDRemovesAFileThatHoldsNothingElse(t *testing.T) {
	for _, body := range []string{"SITREP_HUD_CMD=x\n", "SITREP_HUD_CMD=x", "\nSITREP_HUD_CMD=x\n\n"} {
		c := writeConf(t, body)
		if err := c.DropHUD(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(c.File); !os.IsNotExist(err) {
			t.Errorf("%q left a file", body)
		}
	}
	if err := writeConf(t, "\x00").DropHUD(); err != nil {
		t.Errorf("no file: %v", err)
	}
}

func TestPutHUDCreatesTheFileAndRefusesMoreThanOneLine(t *testing.T) {
	c := writeConf(t, "\x00")
	if err := c.PutHUD("echo hi"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(c.File); string(b) != "SITREP_HUD_CMD='echo hi'\n" {
		t.Errorf("%q", b)
	}
	if err := c.PutHUD("a\nb"); err != ErrMultiline {
		t.Errorf("%v", err)
	}
	if got := c.Get("SITREP_HUD_CMD"); got != "echo hi" {
		t.Errorf("a refused command changed the file: %q", got)
	}
	// a file with no final newline still gets our line on its own line
	c = writeConf(t, "SITREP_QUEUE_ITEMS=3")
	c.PutHUD("x")
	if b, _ := os.ReadFile(c.File); string(b) != "SITREP_QUEUE_ITEMS=3\nSITREP_HUD_CMD='x'\n" {
		t.Errorf("%q", b)
	}
}

// What bash's printf %q writes must read back, and what the Go writer writes must be what bash reads back (the file was sourced by bash).
func TestBashQuotingRoundTrips(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	values := []string{"plain", "with space", `a"b`, "it's", "a'b'c", `back\slash`, "$HOME and $(cmd)", "`tick`", "a && b || c # not a comment", "-leading-dash", "  padded  ",
		"tab\there", "new\nline", "esc\x1b[31m", "bell\a", "é世界 \U0001F600", "semi;colon", "glob*?[x]", "~/tilde", "!bang", "{brace,expansion}", strings.Repeat("x y ", 25000), ""}
	for _, loc := range []string{"C", "en_US.UTF-8", "C.UTF-8"} {
		for _, v := range values {
			cmd := exec.Command("bash", "-c", `printf '%q' "$1"`, "bash", v)
			cmd.Env = []string{"LC_ALL=" + loc, "PATH=" + os.Getenv("PATH")}
			out, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			if got := Unquote(string(out)); got != v {
				t.Errorf("locale %s: bash wrote %q, Unquote gave %q, want %q", loc, out, got, v)
			}
		}
	}
	for _, v := range values {
		if v == "" || strings.ContainsAny(v, "\n") {
			continue
		}
		cmd := exec.Command("bash", "-c", `printf '%s' `+Quote(v))
		out, err := cmd.Output()
		if err != nil || string(out) != v {
			t.Errorf("bash read Quote(%q) as %q (%v)", v, out, err)
		}
	}
}

func TestFromFileIgnoresTheEnvironment(t *testing.T) {
	f := filepath.Join(t.TempDir(), "sitrep.conf")
	os.WriteFile(f, []byte("SITREP_HUD_CMD='from the file'\n"), 0o600)
	c := Conf{File: f, Getenv: func(string) string { return "from the environment" }}
	if c.Get("SITREP_HUD_CMD") != "from the environment" || c.FromFile("SITREP_HUD_CMD") != "from the file" {
		t.Error("FromFile must read the file only")
	}
}

func TestAnsiCEscapes(t *testing.T) {
	for in, want := range map[string]string{
		`$'\n\t\r'`: "\n\t\r", `$'\e[0m'`: "\x1b[0m", `$'\x41\x42'`: "AB", `$'\101'`: "A", `$'é'`: "é", `$'\''`: "'", `$'a\\b'`: `a\b`, `$'\x'`: "x", `$'\q'`: "q", `$'open`: "open",
	} {
		if got := Unquote(in); got != want {
			t.Errorf("Unquote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCheckUpdates(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sitrep.conf")
	os.WriteFile(file, []byte("SITREP_CHECK_UPDATES=0\n"), 0o600)
	env := map[string]string{}
	c := Conf{File: file, Getenv: func(k string) string { return env[k] }}
	if c.CheckUpdates() {
		t.Error("the file turns it off")
	}
	env["SITREP_CHECK_UPDATES"] = "1"
	if !c.CheckUpdates() {
		t.Error("the environment wins")
	}
	for _, v := range []string{"false", "NO", "0"} {
		env["SITREP_CHECK_UPDATES"] = v
		if c.CheckUpdates() {
			t.Errorf("%q leaves it on", v)
		}
	}
	if !(Conf{File: filepath.Join(t.TempDir(), "none"), Getenv: func(string) string { return "" }}).CheckUpdates() {
		t.Error("it is on by default")
	}
}

func TestThemeAndColors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "sitrep.conf")
	os.WriteFile(file, []byte("SITREP_THEME=High-Contrast\nSITREP_COLORS='accent=#ff8800, attn=1/2'\n"), 0o600)
	env := map[string]string{}
	c := Conf{File: file, Getenv: func(k string) string { return env[k] }}
	if c.Theme() != "high-contrast" || c.Colors() != "accent=#ff8800, attn=1/2" {
		t.Errorf("%q %q", c.Theme(), c.Colors())
	}
	env["SITREP_THEME"], env["SITREP_COLORS"] = "mono", "dim=8"
	if c.Theme() != "mono" || c.Colors() != "dim=8" {
		t.Errorf("the environment wins: %q %q", c.Theme(), c.Colors())
	}
	for _, bad := range []string{"mono\x1b[31m", "a b", "x_y", "1", strings.Repeat("a", 21), "é"} {
		env["SITREP_THEME"] = bad
		if c.Theme() != "" {
			t.Errorf("theme %q accepted: %q", bad, c.Theme())
		}
	}
	for _, bad := range []string{"accent=\x1b[31m", "accent=#fff;rm", "a=b\nc", strings.Repeat("a", 301), "accent=é"} {
		env["SITREP_COLORS"] = bad
		if c.Colors() != "" {
			t.Errorf("colors %q accepted", bad)
		}
	}
	empty := Conf{File: filepath.Join(t.TempDir(), "none"), Getenv: func(string) string { return "" }}
	if empty.Theme() != "" || empty.Colors() != "" {
		t.Error("nothing set")
	}
}

func TestTUIInterval(t *testing.T) {
	env := map[string]string{}
	c := Conf{File: filepath.Join(t.TempDir(), "none"), Getenv: func(k string) string { return env[k] }}
	if c.TUIInterval() != 3*time.Second {
		t.Errorf("default %v", c.TUIInterval())
	}
	for in, want := range map[string]time.Duration{"5": 5 * time.Second, "10s": 10 * time.Second, "1m": time.Minute, "1500ms": 1500 * time.Millisecond, "1": time.Second, "86400": 24 * time.Hour} {
		env["SITREP_TUI_INTERVAL"] = in
		if got := c.TUIInterval(); got != want {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
	}
	for _, bad := range []string{"0", "-5", "200ms", "x", "5 s", "1e3", "86401", "999h", "5\x1b[0m", "s"} {
		env["SITREP_TUI_INTERVAL"] = bad
		if got := c.TUIInterval(); got != 3*time.Second {
			t.Errorf("%q gave %v, want the default", bad, got)
		}
	}
	file := filepath.Join(t.TempDir(), "sitrep.conf")
	os.WriteFile(file, []byte("SITREP_TUI_INTERVAL=7\n"), 0o600)
	fc := Conf{File: file, Getenv: func(string) string { return "" }}
	if fc.TUIInterval() != 7*time.Second {
		t.Errorf("from the file: %v", fc.TUIInterval())
	}
}

func TestLogMaxIsCapped(t *testing.T) {
	c := Conf{File: filepath.Join(t.TempDir(), "sitrep.conf"), Getenv: func(k string) string {
		if k == "SITREP_LOG_MAX" {
			return "129"
		}
		return ""
	}}
	if c.LogMaxMB() != 10 {
		t.Fatal("a limit whose half exceeds what a reader takes was accepted")
	}
}

func TestLogSettings(t *testing.T) {
	env := map[string]string{}
	c := Conf{File: filepath.Join(t.TempDir(), "sitrep.conf"), Getenv: func(k string) string { return env[k] }}
	if c.LogOn() || c.LogMaxMB() != 10 || c.LogDays() != 30 {
		t.Fatal("wrong defaults")
	}
	for _, v := range []string{"1", "on", "ON", "true", "yes"} {
		env["SITREP_LOG"] = v
		if !c.LogOn() {
			t.Errorf("SITREP_LOG=%s is not on", v)
		}
	}
	for _, v := range []string{"0", "off", "no", "banana"} {
		env["SITREP_LOG"] = v
		if c.LogOn() {
			t.Errorf("SITREP_LOG=%s is on", v)
		}
	}
	env["SITREP_LOG_MAX"], env["SITREP_LOG_DAYS"] = "50", "7"
	if c.LogMaxMB() != 50 || c.LogDays() != 7 {
		t.Fatal("limits not read")
	}
	for _, v := range []string{"0", "-3", "x", "99999"} {
		env["SITREP_LOG_MAX"], env["SITREP_LOG_DAYS"] = v, v
		if c.LogMaxMB() != 10 || c.LogDays() != 30 {
			t.Errorf("bad limit %q not defaulted", v)
		}
	}
}

func TestAttnFade(t *testing.T) {
	for in, want := range map[string]int{"": 600, "SITREP_ATTN_FADE=0\n": 0, "SITREP_ATTN_FADE=90\n": 90, "SITREP_ATTN_FADE=x\n": 600, "SITREP_ATTN_FADE=-1\n": 600} {
		if got := conf(t, in, nil).AttnFade(); got != want {
			t.Errorf("%q: got %d, want %d", in, got, want)
		}
	}
}

func TestFromFileReadsALineOfAFileWithCRLF(t *testing.T) {
	f := filepath.Join(t.TempDir(), "sitrep.conf")
	os.WriteFile(f, []byte("SITREP_THEME=mono\r\nSITREP_COLORS='dim=7'\r\n"), 0o600)
	c := Conf{File: f, Getenv: func(string) string { return "" }}
	if c.Theme() != "mono" || c.Colors() != "dim=7" {
		t.Errorf("theme %q colours %q", c.Theme(), c.Colors())
	}
}
