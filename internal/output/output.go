package output

import (
	"encoding/json"
	"io"
)

// WriteJSON writes exactly one JSON value followed by a single final newline.
// One call, one value: a caller that needs JSONL calls it once per line.
//
// WriteJSON is a thin, reflection-free encoder: it hands v to encoding/json
// with SetEscapeHTML(false) and serialises exactly what it is given. It
// carries no type rewriting and makes no promise about arbitrary types — a
// value that marshals itself (time.Time, decimal.Decimal) reaches the output
// through its own MarshalJSON, trailing zeros and all. Exact decimals come
// from model.Decimal, whose MarshalJSON keeps the scale the value carries;
// output structs use model.Decimal, never decimal.Decimal directly (the
// package doc records the full contract).
//
// SetEscapeHTML(false) keeps <, > and & verbatim: the values are user-facing
// text (amounts, messages, attribution), not embedded HTML.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
