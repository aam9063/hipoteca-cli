package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// CORE-01.1: a decimal.Decimal must leave the process as its exact decimal
// representation in a JSON string ("742.18"), never as a JSON number.
func TestDecimalSerializesAsString(t *testing.T) {
	type row struct {
		Payment decimal.Decimal `json:"payment"`
		Months  int             `json:"months"`
	}
	var buf bytes.Buffer
	err := WriteJSON(&buf, row{Payment: decimal.New(74218, -2), Months: 293})
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v (raw: %q)", err, buf.String())
	}
	payment, isString := got["payment"].(string)
	if !isString {
		t.Errorf("payment must serialise as a JSON string, got %T: %q", got["payment"], buf.String())
	}
	if payment != "742.18" {
		t.Errorf("payment = %q, want \"742.18\"", payment)
	}
	if !strings.Contains(buf.String(), `"payment":"742.18"`) {
		t.Errorf("raw output must contain the string form, got %q", buf.String())
	}
	if months, ok := got["months"].(float64); !ok || months != 293 {
		t.Errorf("months must stay an integer, got %v", got["months"])
	}
}

// Decimals must keep the scale the value carries (significant trailing zeros,
// arranque §6.4 decision 3 and the "0.890" example) and must never fall into
// scientific notation, whatever their magnitude.
func TestDecimalKeepsScaleAndAvoidsScientificNotation(t *testing.T) {
	type row struct {
		WithZeros decimal.Decimal `json:"with_zeros"`
		Huge      decimal.Decimal `json:"huge"`
	}
	var buf bytes.Buffer
	err := WriteJSON(&buf, row{
		WithZeros: decimal.New(742180, -3), // 742.180
		Huge:      decimal.New(123456789012345, 12),
	})
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if !strings.Contains(buf.String(), `"with_zeros":"742.180"`) {
		t.Errorf("trailing zeros must be preserved, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), `"huge":"123456789012345000000000000"`) {
		t.Errorf("no scientific notation allowed, got %q", buf.String())
	}
}

// Decimals work with no per-call special casing wherever they appear:
// slices, maps and pointers to structs all carry their exact string form.
func TestDecimalsInsideContainers(t *testing.T) {
	type inner struct {
		Rate decimal.Decimal `json:"rate"`
	}
	var buf bytes.Buffer
	err := WriteJSON(&buf, map[string]any{
		"slice":   []decimal.Decimal{decimal.New(310, -2)}, // 3.10
		"pointer": &inner{Rate: decimal.New(890, -3)},      // 0.890
	})
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	raw := buf.String()
	if !strings.Contains(raw, `"slice":["3.10"]`) {
		t.Errorf("slice decimals must be exact strings, got %q", raw)
	}
	if !strings.Contains(raw, `"rate":"0.890"`) {
		t.Errorf("pointer struct decimals must be exact strings, got %q", raw)
	}
}

// A struct that mixes decimals with types that marshal themselves (time.Time)
// keeps both behaviours: the time in its default form, the decimal with scale.
func TestDecimalsCoexistWithSelfMarshallingTypes(t *testing.T) {
	type row struct {
		When  time.Time       `json:"when"`
		Value decimal.Decimal `json:"value"`
	}
	var buf bytes.Buffer
	err := WriteJSON(&buf, row{
		When:  time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		Value: decimal.New(310, -2),
	})
	if err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	raw := buf.String()
	if !strings.Contains(raw, `"when":"2026-12-01T00:00:00Z"`) {
		t.Errorf("time.Time must keep its default JSON form, got %q", raw)
	}
	if !strings.Contains(raw, `"value":"3.10"`) {
		t.Errorf("the decimal must keep its scale next to a time.Time, got %q", raw)
	}
}

// A type the rewriter cannot rebuild (here: an unexported decimal field two
// levels down) must fail loudly when it holds a decimal: left to the default
// marshaler, the decimal would travel through decimal.Decimal's own
// MarshalJSON, which trims trailing zeros and silently breaks the exact-scale
// contract (arranque §6.4, decision 3). The error names the offending type and
// the field path, and nothing is written to the writer.
func TestUnrebuildableDecimalTypeFailsLoudly(t *testing.T) {
	type innerSecret struct {
		rate decimal.Decimal // unexported: the rewriter cannot rebuild this struct
	}
	type row struct {
		Inner innerSecret `json:"inner"`
	}
	var buf bytes.Buffer
	err := WriteJSON(&buf, row{Inner: innerSecret{rate: decimal.New(310, -2)}})
	if err == nil {
		t.Fatal("WriteJSON must fail for a decimal behind a struct it cannot rewrite, got nil")
	}
	var de *DecimalNotExactError
	if !errors.As(err, &de) {
		t.Fatalf("error must be *DecimalNotExactError, got %T: %v", err, err)
	}
	if !strings.Contains(de.TypeName, "innerSecret") {
		t.Errorf("error must name the offending type, got TypeName %q", de.TypeName)
	}
	if de.Path != "Inner" {
		t.Errorf("Path = %q, want \"Inner\" (Go field names, not json tags)", de.Path)
	}
	msg := err.Error()
	if !strings.Contains(msg, "innerSecret") {
		t.Errorf("message must name the offending type, got %q", msg)
	}
	if !strings.Contains(msg, "Inner") {
		t.Errorf("message must name the field path, got %q", msg)
	}
	if buf.Len() != 0 {
		t.Errorf("a failed WriteJSON must write nothing, got %q", buf.String())
	}
}

// A self-referential type holding a decimal cannot be rebuilt either (the
// rebuilt copy would still refer back to the original through the cycle), so
// it must fail loudly with the same error, naming the type.
func TestSelfReferentialDecimalTypeFailsLoudly(t *testing.T) {
	type node struct {
		Next *node           `json:"next"`
		Rate decimal.Decimal `json:"rate"`
	}
	var buf bytes.Buffer
	err := WriteJSON(&buf, node{Rate: decimal.New(890, -3)})
	if err == nil {
		t.Fatal("WriteJSON must fail for a self-referential type holding a decimal, got nil")
	}
	var de *DecimalNotExactError
	if !errors.As(err, &de) {
		t.Fatalf("error must be *DecimalNotExactError, got %T: %v", err, err)
	}
	if !strings.Contains(de.TypeName, "node") {
		t.Errorf("error must name the offending type, got TypeName %q", de.TypeName)
	}
	if buf.Len() != 0 {
		t.Errorf("a failed WriteJSON must write nothing, got %q", buf.String())
	}
}

// A type without decimals keeps falling back to the default marshaler
// silently: unexported fields there cannot lose scale, so nothing needs to be
// loud. encoding/json skips the unexported field as always.
func TestUnrebuildableTypeWithoutDecimalsStillSerialises(t *testing.T) {
	type legacy struct {
		hidden  string
		Visible int `json:"visible"`
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, legacy{hidden: "x", Visible: 7}); err != nil {
		t.Fatalf("types without decimals must keep falling back silently: %v", err)
	}
	if got, want := buf.String(), "{\"visible\":7}\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// omitempty with a zero decimal keeps emitting "0": the rebuilt field is a
// json.RawMessage carrying an explicit value, so encoding/json never treats it
// as empty, and a zero decimal is an explicit value, not an absent one. This
// matches what shopspring's own marshaler emits for a zero; documented here so
// nobody turns it into a silently missing field.
func TestOmitEmptyWithZeroDecimalEmitsZeroString(t *testing.T) {
	type row struct {
		Note  string          `json:"note,omitempty"`
		Extra decimal.Decimal `json:"extra,omitempty"`
	}
	var buf bytes.Buffer
	if err := WriteJSON(&buf, row{Note: "x", Extra: decimal.Zero}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	if got, want := buf.String(), "{\"note\":\"x\",\"extra\":\"0\"}\n"; got != want {
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
