package main

import "testing"

func TestEnvOn(t *testing.T) {
	for v, want := range map[string]bool{"": false, "0": false, "false": false, "FALSE": false, "no": false, "1": true, "yes": true, "true": true, "on": true} {
		t.Setenv("SITREP_TUI_TEST_SWITCH", v)
		if got := envOn("SITREP_TUI_TEST_SWITCH"); got != want {
			t.Errorf("%q: got %v, want %v", v, got, want)
		}
	}
}
