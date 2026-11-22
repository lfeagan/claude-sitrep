package cli

import (
	"errors"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/conf"
	"github.com/lfeagan/claude-sitrep/internal/tui"
)

// colorsFile is sitrep.conf, as the colour tool reads and writes it.
type colorsFile struct {
	c   conf.Conf
	now func() time.Time
}

func (f colorsFile) Path() string { return f.c.Target() }
func (f colorsFile) Values() (string, string) {
	return f.c.FromFile("SITREP_THEME"), f.c.FromFile("SITREP_COLORS")
}
func (f colorsFile) Read() (string, error)              { return f.c.ReadBody() }
func (f colorsFile) Write(out, original string) error   { return f.c.WriteBody(out, original) }
func (f colorsFile) Backup(body string) (string, error) { return f.c.Backup(body, f.now()) }

// cmdColors is the colour tool: a preview of every use of colour, and a picker for each setting.
func cmdColors(e Env, args []string) error {
	fs := e.flags("colors")
	light := fs.Bool("light", false, "preview a light terminal, without asking the terminal which it is")
	dark := fs.Bool("dark", false, "preview a dark terminal, without asking the terminal which it is")
	readOnly := fs.Bool("read-only", envOn(e.conf().Get, "SITREP_TUI_READONLY"), "do not write sitrep.conf (or set SITREP_TUI_READONLY)")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *light && *dark {
		return usageError("usage: sitrep colors [--light|--dark] [--read-only]")
	}
	if !e.isTerminal() {
		return errors.New("sitrep colors needs a terminal")
	}
	opts := tui.ColorsOptions{Sessions: demoSnapshot(e.Clock.Now().Unix()), File: colorsFile{e.conf(), e.Clock.Now}, ReadOnly: *readOnly}
	switch {
	case *light:
		f := false
		opts.Dark = &f
	case *dark:
		t := true
		opts.Dark = &t
	}
	for _, n := range conf.Editable {
		if e.Getenv(n) != "" {
			opts.Notes = append(opts.Notes, n+" is set in the environment, which beats sitrep.conf: a change here shows only where it is unset.")
		}
	}
	if e.Getenv("NO_COLOR") != "" {
		opts.Notes = append(opts.Notes, "NO_COLOR is set: the terminal may show no colour.")
	}
	return tui.RunColors(opts)
}
