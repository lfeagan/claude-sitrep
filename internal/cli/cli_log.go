package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lfeagan/claude-sitrep/internal/histlog"
	"github.com/lfeagan/claude-sitrep/internal/queue"
	"github.com/lfeagan/claude-sitrep/internal/state"
	"github.com/lfeagan/claude-sitrep/internal/textsafe"
)

// since reads --since: a length of time back from now (90m, 36h, 2d) or a date (2026-11-21, or a full RFC 3339 time).
func since(s string, now time.Time) (time.Time, error) {
	if n, ok := strings.CutSuffix(s, "d"); ok {
		if d, err := strconv.Atoi(n); err == nil && d >= 0 && d <= 36500 {
			return now.Add(-time.Duration(d) * 24 * time.Hour), nil
		}
	}
	if d, err := time.ParseDuration(s); err == nil && d >= 0 {
		return now.Add(-d), nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("--since wants a length of time such as 90m, 36h or 2d, or a date such as 2026-11-21")
}

// logJSON is an entry as --json prints it.
type logJSON struct {
	E    string        `json:"e"`
	T    string        `json:"t"`
	Host string        `json:"host"`
	ID   state.ID      `json:"id"`
	Op   string        `json:"op"`
	Text textsafe.Text `json:"text,omitempty"`
	Old  textsafe.Text `json:"old,omitempty"`
	From int           `json:"from,omitempty"`
	To   int           `json:"to,omitempty"`
	Dest state.ID      `json:"dest,omitempty"`
}

func cmdLog(e Env, args []string) error {
	fs := e.flags("log")
	sess := fs.String("session", "", "only this session or queue (an id, a prefix, an alias, or inbox)")
	op := fs.String("op", "", "only this operation, such as queue.add")
	host := fs.String("host", "", "only this host")
	sinceFlag := fs.String("since", "", "only entries newer than this: 90m, 36h, 2d or a date")
	n := fs.Int("n", 50, "show the latest this many (0 shows all)")
	asJSON := fs.Bool("json", false, "print one JSON object per line")
	wipe := fs.Bool("clear", false, "delete this host's log files")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	l := e.log()
	if *wipe {
		if err := l.Clear(); err != nil {
			return fmt.Errorf("could not remove all of the log: %v", err)
		}
		fmt.Fprintln(e.Stdout, "log cleared")
		return nil
	}
	now := e.Clock.Now()
	q := histlog.Query{Host: *host, Op: *op, Days: e.conf().LogDays(), Now: now, Limit: *n}
	if *n < 0 {
		return errors.New("-n must not be negative")
	}
	if *op != "" && !histlog.KnownOp(*op) {
		return fmt.Errorf("unknown operation '%s'", textsafe.Clean(*op, 40))
	}
	if *sinceFlag != "" {
		t, err := since(*sinceFlag, now)
		if err != nil {
			return err
		}
		q.Since = t
	}
	if *sess != "" {
		id, err := e.resolve(*sess)
		if err != nil {
			if !strings.Contains(err.Error(), "matches 0;") {
				return err // an ambiguous prefix stays an error
			}
			if id, err = state.ParseID(*sess); err != nil { // a session that is gone can still have a past
				return err
			}
		}
		q.ID = id
	}
	items := histlog.Read(e.store().Dir, q)
	if len(items) == 0 {
		fmt.Fprintln(e.Stdout, "no entries")
		if !l.On {
			fmt.Fprintln(e.Stdout, "the history log is off; set SITREP_LOG=on in sitrep.conf to keep one")
		}
		return nil
	}
	for _, it := range items {
		if *asJSON {
			b, err := json.Marshal(logJSON{E: it.E, T: it.Time.UTC().Format(time.RFC3339), Host: it.Host, ID: it.ID, Op: it.Op, Text: it.Text, Old: it.Old, From: it.From, To: it.To, Dest: it.Dest})
			if err == nil {
				fmt.Fprintln(e.Stdout, string(b))
			}
			continue
		}
		fmt.Fprintln(e.Stdout, strings.TrimRight(fmt.Sprintf("%s  %s  %-5s  %-13s  %s", it.Time.UTC().Format("2006-01-02 15:04Z"), it.Host, queue.Label(it.ID), it.Op, describe(it)), " "))
	}
	return nil
}

// describe is what an entry says, after its operation.
func describe(it histlog.Item) string {
	switch it.Op {
	case histlog.QueueMove:
		return fmt.Sprintf("-> %s: %s", queue.Label(it.Dest), it.Text)
	case histlog.QueueReorder:
		return fmt.Sprintf("%d -> %d: %s", it.From, it.To, it.Text)
	case histlog.QueueEdit:
		return fmt.Sprintf("%s -> %s", it.Old, it.Text)
	case histlog.Alias:
		switch {
		case it.Text.String() == "":
			return fmt.Sprintf("%s removed", it.Old)
		case it.Old.String() == "":
			return it.Text.String()
		}
		return fmt.Sprintf("%s -> %s", it.Old, it.Text)
	}
	return it.Text.String()
}
