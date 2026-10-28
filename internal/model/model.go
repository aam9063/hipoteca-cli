// Package model defines the output structures and their schema_version. With schemas/
// it is the contract with agents: changes go in their own PR (P-09).
package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// SchemaVersion is the version of every output contract (P-09). Fields are
// additive only within a version; a breaking change bumps this constant in its
// own PR and updates schemas/.
const SchemaVersion = 1

// Decimal is the exact-decimal type of the output contract (arranque §6.4,
// decision 3). It wraps shopspring's decimal.Decimal and serialises as a JSON
// string carrying the exact decimal representation, keeping the scale the
// value carries ("0.890" stays "0.890") and never using scientific notation.
// Structs of the output contract use Decimal for every amount and rate; a raw
// decimal.Decimal field is forbidden (see nofloat_test.go) because shopspring's
// own MarshalJSON trims trailing zeros and would silently lose scale.
//
// The zero value is usable and serialises as "0".
type Decimal struct {
	v decimal.Decimal
}

// NewDecimal parses s as an exact decimal string ("742.18", "0.890", "-3.10").
func NewDecimal(s string) (Decimal, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return Decimal{}, fmt.Errorf("decimal inválido %q: %w", s, err)
	}
	return Decimal{v: d}, nil
}

// FromDecimal wraps an existing decimal.Decimal.
func FromDecimal(d decimal.Decimal) Decimal {
	return Decimal{v: d}
}

// Decimal returns the underlying decimal.Decimal, for callers that need the
// shopspring API (comparisons, arithmetic) without exposing the field.
func (d Decimal) Decimal() decimal.Decimal {
	return d.v
}

// String renders the exact decimal representation, the same text the JSON form
// carries: StringFixed at the value's own exponent never changes the value, it
// only keeps the zeros that decimal.String() would trim.
func (d Decimal) String() string {
	return d.v.StringFixed(-d.v.Exponent())
}

// MarshalJSON emits the exact decimal representation as a JSON string, with the
// scale the value carries and no scientific notation.
func (d Decimal) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

// UnmarshalJSON accepts the exact-scale string back (and, for convenience, a
// bare JSON number, as encoding/json may hand one over when decoding into an
// interface before re-marshalling).
func (d *Decimal) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	v, err := decimal.NewFromString(s)
	if err != nil {
		return fmt.Errorf("decimal inválido %s: %w", b, err)
	}
	d.v = v
	return nil
}

// The two attribution texts of P-05 (arranque §6.2). They live here, not in the
// commands, so the exact wording has a single source.
const (
	attributionRawData   = "Fuente: Sitio web del Banco de España (www.bde.es)"
	attributionProcessed = "Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)"
)

// updatedAtLayout is the only accepted shape of data_updated_at (P-05): the
// date the source published, exactly as the Banco de España renders it.
const updatedAtLayout = "2006-01-02"

// Attribution is the attribution block that every output carrying Banco de
// España data or calculations over it must include (P-05).
//
// The fields are exported only so encoding/json can see them. RawDataAttribution
// and ProcessedAttribution are the only supported way to build one: they
// guarantee the exact text, that NotEndorsed is always true and that a
// well-formed data_updated_at is carried. Do not build an Attribution literal
// elsewhere — a struct literal can skip those invariants, and a test in this
// package pins what the constructors produce.
type Attribution struct {
	Text          string `json:"text"`
	DataUpdatedAt string `json:"data_updated_at"`
	NotEndorsed   bool   `json:"not_endorsed"`
}

// RawDataAttribution returns the attribution for outputs with untouched Banco
// de España data (index list/show/latest). updatedAt is the last update date of
// the data, as published by the source, in YYYY-MM-DD form; anything else is
// rejected with an error and a zero Attribution.
func RawDataAttribution(updatedAt string) (Attribution, error) {
	return newAttribution(attributionRawData, updatedAt)
}

// ProcessedAttribution returns the attribution for outputs built from our own
// calculations over Banco de España data (revision, schedule, amortize,
// scenario, compare). updatedAt must be a YYYY-MM-DD date; anything else is
// rejected with an error and a zero Attribution.
func ProcessedAttribution(updatedAt string) (Attribution, error) {
	return newAttribution(attributionProcessed, updatedAt)
}

func newAttribution(text, updatedAt string) (Attribution, error) {
	if _, err := time.Parse(updatedAtLayout, updatedAt); err != nil {
		return Attribution{}, fmt.Errorf("data_updated_at %q no es una fecha YYYY-MM-DD: %w", updatedAt, err)
	}
	return Attribution{
		Text:          text,
		DataUpdatedAt: updatedAt,
		NotEndorsed:   true,
	}, nil
}
