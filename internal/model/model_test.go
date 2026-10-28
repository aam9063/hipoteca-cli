package model

import (
	"encoding/json"
	"testing"
)

// P-05: the two attribution texts are exact, with a data_updated_at and
// not_endorsed always true. The constructors are the only supported way to
// build an Attribution: this test is the assertion that one built through them
// always carries a date and the not-endorsed flag.
func TestAttributionTextsAreExact(t *testing.T) {
	const updatedAt = "2026-12-01"

	raw := RawDataAttribution(updatedAt)
	if raw.Text != "Fuente: Sitio web del Banco de España (www.bde.es)" {
		t.Errorf("raw data text = %q", raw.Text)
	}
	if raw.DataUpdatedAt != updatedAt {
		t.Errorf("raw data data_updated_at = %q, want %q", raw.DataUpdatedAt, updatedAt)
	}
	if !raw.NotEndorsed {
		t.Error("not_endorsed must be true")
	}

	processed := ProcessedAttribution(updatedAt)
	if processed.Text != "Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)" {
		t.Errorf("processed text = %q", processed.Text)
	}
	if processed.DataUpdatedAt != updatedAt {
		t.Errorf("processed data_updated_at = %q, want %q", processed.DataUpdatedAt, updatedAt)
	}
	if !processed.NotEndorsed {
		t.Error("not_endorsed must be true")
	}
}

// The attribution block serialises with exactly the contract keys of §6.2.
func TestAttributionJSONShape(t *testing.T) {
	raw, err := json.Marshal(ProcessedAttribution("2026-12-01"))
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("attribution is not JSON: %v (raw: %s)", err, raw)
	}
	want := map[string]any{
		"text":            "Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)",
		"data_updated_at": "2026-12-01",
		"not_endorsed":    true,
	}
	if len(got) != len(want) {
		t.Errorf("attribution keys = %v, want exactly %v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("attribution[%q] = %v, want %v", key, got[key], value)
		}
	}
}

func TestSchemaVersionIsOne(t *testing.T) {
	if SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", SchemaVersion)
	}
}
