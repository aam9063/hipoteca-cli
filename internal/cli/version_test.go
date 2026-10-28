package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

// The skeleton only ships `version`, but it already pins the output contract that
// every command must honour (P-01): exactly one JSON value on stdout, nothing else.
func TestVersionEmitsSingleJSONValue(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "test", Commit: "abc1234", Date: "2026-10-28"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version failed: %v", err)
	}

	dec := json.NewDecoder(bytes.NewReader(out.Bytes()))
	var got struct {
		SchemaVersion int    `json:"schema_version"`
		Version       string `json:"version"`
		Commit        string `json:"commit"`
		Date          string `json:"date"`
	}
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("stdout is not JSON: %v (raw: %q)", err, out.String())
	}
	if dec.More() {
		t.Fatalf("stdout carries more than one JSON value: %q", out.String())
	}
	if got.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", got.SchemaVersion)
	}
	if got.Version != "test" {
		t.Errorf("version = %q, want the injected value", got.Version)
	}
	if got.Commit == "" || got.Date == "" {
		t.Errorf("commit/date must be present (got %q/%q)", got.Commit, got.Date)
	}
}
