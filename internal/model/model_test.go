package model

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
)

// P-05: the two attribution texts are exact, with a data_updated_at and
// not_endorsed always true. The constructors are the only supported way to
// build an Attribution: this test is the assertion that one built through them
// always carries a date and the not-endorsed flag.
func TestAttributionTextsAreExact(t *testing.T) {
	const updatedAt = "2026-12-01"

	raw, err := RawDataAttribution(updatedAt)
	if err != nil {
		t.Fatalf("RawDataAttribution(%q) failed: %v", updatedAt, err)
	}
	if raw.Text != "Fuente: Sitio web del Banco de España (www.bde.es)" {
		t.Errorf("raw data text = %q", raw.Text)
	}
	if raw.DataUpdatedAt != updatedAt {
		t.Errorf("raw data data_updated_at = %q, want %q", raw.DataUpdatedAt, updatedAt)
	}
	if !raw.NotEndorsed {
		t.Error("not_endorsed must be true")
	}

	processed, err := ProcessedAttribution(updatedAt)
	if err != nil {
		t.Fatalf("ProcessedAttribution(%q) failed: %v", updatedAt, err)
	}
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

// The constructors are the only supported way to build an Attribution, so they
// must refuse to build one without a well-formed data_updated_at (P-05, review
// finding 7): an empty or unparseable date returns an error and a zero
// Attribution, so an invalid attribution can never reach the output contract.
func TestAttributionRejectsInvalidUpdatedAt(t *testing.T) {
	cases := map[string]string{
		"empty":           "",
		"not a date":      "ayer",
		"no zero padding": "2026-12-1",
		"impossible date": "2026-13-40",
		"full datetime":   "2026-12-01T00:00:00Z",
	}
	for name, updatedAt := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := RawDataAttribution(updatedAt)
			if err == nil {
				t.Fatalf("RawDataAttribution(%q) must fail, got %+v", updatedAt, raw)
			}
			if raw != (Attribution{}) {
				t.Errorf("RawDataAttribution(%q) must return a zero Attribution on error, got %+v", updatedAt, raw)
			}
			processed, err := ProcessedAttribution(updatedAt)
			if err == nil {
				t.Fatalf("ProcessedAttribution(%q) must fail, got %+v", updatedAt, processed)
			}
			if processed != (Attribution{}) {
				t.Errorf("ProcessedAttribution(%q) must return a zero Attribution on error, got %+v", updatedAt, processed)
			}
		})
	}
}

// The attribution block serialises with exactly the contract keys of §6.2.
func TestAttributionJSONShape(t *testing.T) {
	attr, err := ProcessedAttribution("2026-12-01")
	if err != nil {
		t.Fatalf("ProcessedAttribution failed: %v", err)
	}
	raw, err := json.Marshal(attr)
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

// model.Decimal is the exact-decimal type of the output contract (decision 3,
// review redesign): its MarshalJSON emits the exact decimal representation as a
// JSON string, keeping the scale the value carries, and never falls into
// scientific notation.
func TestDecimalMarshalJSONKeepsExactScale(t *testing.T) {
	cases := []struct {
		name string
		d    Decimal
		want string
	}{
		{"plain amount", FromDecimal(decimal.New(74218, -2)), "742.18"},
		{"trailing zeros kept", FromDecimal(decimal.New(742180, -3)), "742.180"},
		{"huge, no scientific notation", FromDecimal(decimal.New(123456789012345, 12)), "123456789012345000000000000"},
		{"negative", FromDecimal(decimal.New(-310, -2)), "-3.10"},
		{"zero value of the type", Decimal{}, "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.d)
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}
			if got := string(raw); got != `"`+tc.want+`"` {
				t.Errorf("MarshalJSON = %s, want %q", got, tc.want)
			}
		})
	}
}

// NewDecimal parses the exact decimal representation a value carries, so a
// round-trip through JSON keeps the scale ("3.10" stays "3.10", not "3.1").
func TestDecimalJSONRoundTripKeepsScale(t *testing.T) {
	d, err := NewDecimal("3.10")
	if err != nil {
		t.Fatalf("NewDecimal failed: %v", err)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if got, want := string(raw), `"3.10"`; got != want {
		t.Fatalf("marshalled = %s, want %s", got, want)
	}
	var back Decimal
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if back.Decimal().StringFixed(-back.Decimal().Exponent()) != "3.10" {
		t.Errorf("round-trip lost the scale: %q", back.Decimal().String())
	}
	if !back.Decimal().Equal(d.Decimal()) {
		t.Errorf("round-trip changed the value: %v != %v", back.Decimal(), d.Decimal())
	}
}

// UnmarshalJSON accepts the exact-scale string back.
func TestDecimalUnmarshalJSONAcceptsExactString(t *testing.T) {
	var d Decimal
	if err := json.Unmarshal([]byte(`"0.890"`), &d); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if got := d.Decimal().StringFixed(-d.Decimal().Exponent()); got != "0.890" {
		t.Errorf("unmarshalled = %q, want \"0.890\"", got)
	}
}

// NewDecimal rejects input that is not an exact decimal string.
func TestNewDecimalRejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "abc", "1.2.3", "742,18"} {
		d, err := NewDecimal(s)
		if err == nil {
			t.Errorf("NewDecimal(%q) must fail, got %v", s, d)
		}
		if d.Decimal().IsZero() && d != (Decimal{}) {
			t.Errorf("NewDecimal(%q) must return a usable zero value on error, got %+v", s, d)
		}
	}
}

// String() renders the exact decimal representation, the same text the JSON
// form carries.
func TestDecimalStringKeepsScale(t *testing.T) {
	d, err := NewDecimal("0.890")
	if err != nil {
		t.Fatalf("NewDecimal failed: %v", err)
	}
	if got := d.String(); got != "0.890" {
		t.Errorf("String() = %q, want \"0.890\"", got)
	}
	zero := Decimal{}
	if got := zero.String(); got != "0" {
		t.Errorf("zero value String() = %q, want \"0\"", got)
	}
}

// FromDecimal keeps the underlying decimal.Decimal reachable for callers that
// need the shopspring API (comparisons, arithmetic) without exporting the field.
func TestFromDecimalExposesUnderlying(t *testing.T) {
	want := decimal.New(310, -2)
	d := FromDecimal(want)
	if !d.Decimal().Equal(want) {
		t.Errorf("Decimal() = %v, want %v", d.Decimal(), want)
	}
}
