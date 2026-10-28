package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/shopspring/decimal"
)

// CORE-01.1 (P-01): a decimal of the contract type leaves the process as its
// exact decimal representation in a JSON string ("742.18"), never as a JSON
// number. Exactness comes from model.Decimal: WriteJSON serialises what it is
// given, and model.Decimal's MarshalJSON keeps the scale the value carries.
func TestWriteJSONEmitsContractDecimalsAsString(t *testing.T) {
	type row struct {
		Payment model.Decimal `json:"payment"`
		Months  int           `json:"months"`
	}
	payment, err := model.NewDecimal("742.18")
	if err != nil {
		t.Fatalf("NewDecimal failed: %v", err)
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, row{Payment: payment, Months: 293}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v (raw: %q)", err, buf.String())
	}
	if payment, ok := got["payment"].(string); !ok || payment != "742.18" {
		t.Errorf("payment must serialise as the exact JSON string, got %v (%q)", got["payment"], buf.String())
	}
	if months, ok := got["months"].(float64); !ok || months != 293 {
		t.Errorf("months must stay an integer, got %v", got["months"])
	}
}

// model.Decimal keeps the scale the value carries (significant trailing zeros,
// arranque §6.4 decision 3, the "0.890" example) and never falls into
// scientific notation, whatever its magnitude.
func TestWriteJSONKeepsDecimalScaleAndAvoidsScientificNotation(t *testing.T) {
	type row struct {
		WithZeros model.Decimal `json:"with_zeros"`
		Huge      model.Decimal `json:"huge"`
	}
	withZeros, err := model.NewDecimal("742.180")
	if err != nil {
		t.Fatalf("NewDecimal failed: %v", err)
	}
	huge, err := model.NewDecimal("123456789012345000000000000")
	if err != nil {
		t.Fatalf("NewDecimal failed: %v", err)
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, row{WithZeros: withZeros, Huge: huge}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	raw := buf.String()
	if !strings.Contains(raw, `"with_zeros":"742.180"`) {
		t.Errorf("trailing zeros must be preserved, got %q", raw)
	}
	if !strings.Contains(raw, `"huge":"123456789012345000000000000"`) {
		t.Errorf("no scientific notation allowed, got %q", raw)
	}
}

// The zero value of model.Decimal serialises as "0": a zero decimal is an
// explicit value, not an absent one, even with omitempty.
func TestWriteJSONZeroDecimalEmitsZeroString(t *testing.T) {
	type row struct {
		Note  string        `json:"note,omitempty"`
		Extra model.Decimal `json:"extra,omitempty"`
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, row{Note: "x"}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if got, want := buf.String(), "{\"note\":\"x\",\"extra\":\"0\"}\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// Review finding 1: WriteJSON serialises exactly what it is given. There is no
// reflective rewriting left, so a decimal.Decimal behind a non-empty interface
// (or anywhere else) reaches the output through that value's own MarshalJSON,
// byte for byte what encoding/json would produce — including its trailing-zero
// trimming. WriteJSON neither detects nor fixes that: the writer no longer
// promises to repair arbitrary structs. Exact decimals come from model.Decimal.
func TestWriteJSONSerialisesWhatItIsGiven(t *testing.T) {
	type row struct {
		Legacy fmt.Stringer  `json:"legacy"` // non-empty interface holding a raw decimal.Decimal
		Exact  model.Decimal `json:"exact"`
	}
	exact, err := model.NewDecimal("3.10")
	if err != nil {
		t.Fatalf("NewDecimal failed: %v", err)
	}
	v := row{Legacy: decimal.New(310, -2), Exact: exact}

	var buf bytes.Buffer
	if err := WriteJSON(&buf, v); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	// Byte for byte what a plain encoder produces for the same value: the
	// passthrough is the contract, the trim is decimal.Decimal's own doing.
	var plain bytes.Buffer
	if err := json.NewEncoder(&plain).Encode(v); err != nil {
		t.Fatalf("plain encode failed: %v", err)
	}
	if got := buf.String(); got != plain.String() {
		t.Errorf("WriteJSON output = %q, want the plain encoder output %q", got, plain.String())
	}
	if !strings.Contains(buf.String(), `"legacy":"3.1"`) {
		t.Errorf("the raw decimal must reach the output untouched by WriteJSON, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), `"exact":"3.10"`) {
		t.Errorf("model.Decimal must keep the scale the value carries, got %q", buf.String())
	}
}

// Review finding 2: a nil pointer field serialises as null, with no panic —
// encoding/json handles nil pointers; the writer needs no special casing.
func TestWriteJSONNilPointerFieldSerialisesAsNull(t *testing.T) {
	type inner struct {
		Rate model.Decimal `json:"rate"`
	}
	type row struct {
		Inner *inner `json:"inner"`
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, row{}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if got, want := buf.String(), "{\"inner\":null}\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// Review finding 2: a nil root value serialises as null, with no panic.
func TestWriteJSONNilRootSerialisesAsNull(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, (*struct {
		A int `json:"a"`
	})(nil)); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if got, want := buf.String(), "null\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// SetEscapeHTML(false): <, > and & in user-facing text (messages, attribution)
// stay verbatim instead of becoming \u003c and friends.
func TestWriteJSONKeepsHTMLEscapableCharactersVerbatim(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSON(&buf, map[string]string{"text": "<b>&</b>"}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if got, want := buf.String(), "{\"text\":\"<b>&</b>\"}\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// P-01: one WriteJSON call emits exactly one complete JSON value, so two calls
// produce two lines and nothing else: no trailing garbage after the last newline.
func TestTwoWritesProduceTwoCompleteJSONValues(t *testing.T) {
	type row struct {
		SchemaVersion int `json:"schema_version"`
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, row{SchemaVersion: 1}); err != nil {
		t.Fatalf("first WriteJSON failed: %v", err)
	}
	if err := WriteJSON(&buf, row{SchemaVersion: 2}); err != nil {
		t.Fatalf("second WriteJSON failed: %v", err)
	}

	raw := buf.String()
	if !strings.HasSuffix(raw, "\n") {
		t.Errorf("output must end with a newline, got %q", raw)
	}
	if strings.HasSuffix(raw, "\n\n") {
		t.Errorf("no blank line after the value, got %q", raw)
	}
	if n := strings.Count(raw, "\n"); n != 2 {
		t.Errorf("two values must produce exactly two lines, got %d newlines: %q", n, raw)
	}

	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	first := row{}
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("first value is not JSON: %v", err)
	}
	if first.SchemaVersion != 1 {
		t.Errorf("first value schema_version = %d, want 1", first.SchemaVersion)
	}
	second := row{}
	if err := dec.Decode(&second); err != nil {
		t.Fatalf("second value is not JSON: %v", err)
	}
	if second.SchemaVersion != 2 {
		t.Errorf("second value schema_version = %d, want 2", second.SchemaVersion)
	}
	if dec.More() {
		t.Errorf("no third value may follow, got %q", raw)
	}
}
