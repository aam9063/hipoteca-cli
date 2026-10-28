// Package output formats results: JSON by default (P-01), pretty es-ES formatting on
// request.
//
// # What WriteJSON promises
//
// WriteJSON writes exactly one JSON value with a single final newline: one call,
// one value, two calls, two lines. It escapes nothing for HTML (SetEscapeHTML(false)),
// so <, > and & in user-facing text stay verbatim. It is the only JSON writer of the
// CLI: every command result and every error envelope goes through it, so the output
// has a single encoder and a single set of guarantees.
//
// # What WriteJSON does not promise
//
// WriteJSON serialises what it is given. There is no reflective type rewriting and
// no fallback: a value that marshals itself (time.Time, decimal.Decimal) reaches the
// output through its own MarshalJSON, untouched. In particular a raw decimal.Decimal
// sent directly — as a field, behind a non-empty interface, or as the root — is
// rendered by shopspring's own zero-trimming marshaler ("3.10" would leave as "3.1"),
// and WriteJSON will neither detect nor fix that: the writer no longer promises to
// repair arbitrary structs. Exact decimals come from model.Decimal: structs of the
// output contract use model.Decimal, never decimal.Decimal directly, and an AST test
// in internal/model fails if a raw decimal.Decimal field appears in that package.
//
// # Explicit help is the one text exception
//
// The CLI's results are JSON (P-01). Explicit help — `hipoteca --help`, `hipoteca
// help`, `hipoteca version --help` — is prose for a human, written to stdout with
// exit 0: a person piping help to a pager is the normal case, and that text is not
// meant to be parsed. This is decision 7 of CORE-01, the documented exception to
// "stdout carries a JSON value".
package output
