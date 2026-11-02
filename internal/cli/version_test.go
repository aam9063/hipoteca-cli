package cli

import (
	"bytes"
	"encoding/json"
	"testing"
)

// `version` imprime sólo la versión: es lo que alguien espera del comando y lo
// que consume install.sh. El commit y la fecha son para diagnóstico y viven en
// `doctor` o detrás de --json.
func TestVersionPrintsOnlyTheVersion(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "0.1.0", Commit: "abc1234", Date: "2026-10-28"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version failed: %v", err)
	}
	if got := out.String(); got != "0.1.0\n" {
		t.Fatalf("salida = %q, se esperaba sólo la versión y un salto", got)
	}
}

// --json mantiene el contrato completo (P-01, P-09): un único valor JSON con
// schema_version, versión, commit y fecha.
func TestVersionJSONKeepsTheContract(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Version: "0.1.0", Commit: "abc1234", Date: "2026-10-28"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version --json failed: %v", err)
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
	if got.Version != "0.1.0" {
		t.Errorf("version = %q, want the injected value", got.Version)
	}
	if got.Commit == "" || got.Date == "" {
		t.Errorf("commit/date must be present (got %q/%q)", got.Commit, got.Date)
	}
}
