// Package model defines the output structures and their schema_version. With schemas/
// it is the contract with agents: changes go in their own PR (P-09).
package model

// SchemaVersion is the version of every output contract (P-09). Fields are
// additive only within a version; a breaking change bumps this constant in its
// own PR and updates schemas/.
const SchemaVersion = 1

// The two attribution texts of P-05 (arranque §6.2). They live here, not in the
// commands, so the exact wording has a single source.
const (
	attributionRawData   = "Fuente: Sitio web del Banco de España (www.bde.es)"
	attributionProcessed = "Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)"
)

// Attribution is the attribution block that every output carrying Banco de
// España data or calculations over it must include (P-05).
//
// The fields are exported only so encoding/json can see them. RawDataAttribution
// and ProcessedAttribution are the only supported way to build one: they
// guarantee the exact text, that NotEndorsed is always true and that a
// data_updated_at is carried. Do not build an Attribution literal elsewhere —
// a struct literal can skip those invariants, and a test in this package pins
// what the constructors produce.
type Attribution struct {
	Text          string `json:"text"`
	DataUpdatedAt string `json:"data_updated_at"`
	NotEndorsed   bool   `json:"not_endorsed"`
}

// RawDataAttribution returns the attribution for outputs with untouched Banco
// de España data (index list/show/latest). updatedAt is the last update date of
// the data, as published by the source.
func RawDataAttribution(updatedAt string) Attribution {
	return newAttribution(attributionRawData, updatedAt)
}

// ProcessedAttribution returns the attribution for outputs built from our own
// calculations over Banco de España data (revision, schedule, amortize,
// scenario, compare).
func ProcessedAttribution(updatedAt string) Attribution {
	return newAttribution(attributionProcessed, updatedAt)
}

func newAttribution(text, updatedAt string) Attribution {
	return Attribution{
		Text:          text,
		DataUpdatedAt: updatedAt,
		NotEndorsed:   true,
	}
}
