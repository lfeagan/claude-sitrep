package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFixture(t *testing.T) Snapshot {
	t.Helper()
	f, err := os.Open("testdata/snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s, err := decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDecodeFixture(t *testing.T) {
	s := loadFixture(t)
	if len(s.Sessions) != 4 || s.Registry != "ok" || s.Inbox.Count != 3 || s.Hidden.Ended != 1 {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
	if s.Sessions[2].QuietSecs != nil {
		t.Errorf("a null quiet_secs must stay nil")
	}
	if s.Sessions[1].QuietSecs == nil || *s.Sessions[1].QuietSecs != 1500 {
		t.Errorf("quiet_secs lost")
	}
}

func TestDecodeCleansText(t *testing.T) {
	s := loadFixture(t)
	for _, x := range s.Sessions {
		for _, str := range []string{x.Objective, x.Progress, x.Now, x.Dir, x.StateLabel} {
			if clean(str) != str {
				t.Errorf("text was not cleaned: %q", str)
			}
		}
	}
	if got := s.Sessions[2].Objective; strings.ContainsAny(got, "\x1b\a\t") || strings.ContainsRune(got, 0x202e) || strings.ContainsRune(got, 0x2028) {
		t.Errorf("hostile characters survived: %q", got)
	}
}

func TestDecodeVersion(t *testing.T) {
	for _, v := range []int{0, 2, 99} {
		_, err := decode(strings.NewReader(fmt.Sprintf(`{"version":%d,"sessions":[]}`, v)))
		if err == nil || !strings.Contains(err.Error(), "version") {
			t.Errorf("version %d should be refused, got %v", v, err)
		}
	}
}

func TestDecodeToleratesNewFields(t *testing.T) {
	s, err := decode(strings.NewReader(`{"version":1,"future":{"a":1},"sessions":[{"id":"x","running":true,"state":"waiting","extra":[1,2]}]}`))
	if err != nil || len(s.Sessions) != 1 {
		t.Fatalf("unknown fields must be ignored: %v", err)
	}
}

func TestDecodeSanityLimits(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"version":1,"registry":"weird","sessions":[`)
	for i := 0; i < maxSessions+50; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":"s%d","state":"bogus","age":-5,"queue":{"count":0,"items":["a"]}}`, i)
	}
	b.WriteString(`]}`)
	s, err := decode(strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Sessions) != maxSessions {
		t.Errorf("sessions not capped: %d", len(s.Sessions))
	}
	if s.Registry != "unavailable" || s.Sessions[0].State != "-" || s.Sessions[0].Age != 0 {
		t.Errorf("values not normalised: %+v", s.Sessions[0])
	}
	if s.Sessions[0].Queue.Count != 1 {
		t.Errorf("count must be at least the items listed")
	}
}

func TestDecodeBadInput(t *testing.T) {
	for _, in := range []string{"", "not json", `{"version":1,"sessions":[`, "[]"} {
		if _, err := decode(strings.NewReader(in)); err == nil {
			t.Errorf("%q should fail", in)
		}
	}
}

// script writes an executable shell script into dir and returns its path.
func script(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDecodeClampsCountsAndQueues(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"version":1,"hidden":{"ended":-4,"stranded_items":-1},"sessions":[{"id":"a","queue":{"count":150,"items":[`)
	for i := 0; i < 150; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`"x"`)
	}
	b.WriteString(`]}}]}`)
	s, err := decode(strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(s.Sessions[0].Queue.Items); got != maxItems {
		t.Errorf("items not capped: %d", got)
	}
	if s.Sessions[0].Queue.Count != 150 {
		t.Errorf("the true count must survive the cap: %d", s.Sessions[0].Queue.Count)
	}
	if s.Hidden.Ended != 0 || s.Hidden.StrandedItems != 0 {
		t.Errorf("negative counts must be clamped: %+v", s.Hidden)
	}
}
