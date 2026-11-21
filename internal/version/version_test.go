package version

import (
	"encoding/json"
	"os"
	"testing"
)

func TestMatchesThePluginManifest(t *testing.T) {
	b, err := os.ReadFile("../../.claude-plugin/plugin.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Version != Version {
		t.Errorf("plugin.json says %s and the program says %s: bump both", m.Version, Version)
	}
}
