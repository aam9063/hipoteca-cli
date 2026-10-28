// Package errs defines the error envelope and the stable exit codes of the arranque,
// section 6.4 (P-02). No exit code is invented outside this package.
package errs

import (
	"errors"
	"fmt"
)

// Kind identifies one of the error types of the contract (arranque §6.4). The
// zero value is deliberately not a valid kind: errors are always built with an
// explicit kind.
type Kind string

// The named error types of the table in §6.4. The interrupted-by-signal case
// (130) is not an envelope type: it never carries one (see CodeInterrupted).
const (
	KindInternal             Kind = "internal"
	KindUsage                Kind = "usage"
	KindLoanIncomplete       Kind = "loan_incomplete"
	KindCancelled            Kind = "cancelled"
	KindConfirmationRequired Kind = "confirmation_required"
	KindNotFound             Kind = "not_found"
	KindIndexNotPublished    Kind = "index_not_published"
	KindUpstreamUnavailable  Kind = "upstream_unavailable"
	KindRateLimited          Kind = "rate_limited"
	KindNetwork              Kind = "network"
	KindAPIChanged           Kind = "api_changed"
)

// CodeInterrupted is the exit code when the process is interrupted by a signal
// (Ctrl+C, §6.4). It is not in the type table: there is no envelope for it.
const CodeInterrupted = 130

// exitCodes is the single source of the type → code table (§6.4). Every other
// package must come here through ExitCode, never with its own mapping.
var exitCodes = map[Kind]int{
	KindInternal:             1,
	KindUsage:                2,
	KindLoanIncomplete:       2,
	KindCancelled:            2,
	KindConfirmationRequired: 2,
	KindNotFound:             4,
	KindIndexNotPublished:    4,
	KindUpstreamUnavailable:  5,
	KindRateLimited:          5,
	KindNetwork:              6,
	KindAPIChanged:           7,
}

// ExitCode returns the stable exit code for kind. An unknown kind falls into
// internal (code 1): no new code is ever invented here.
func ExitCode(kind Kind) int {
	if code, ok := exitCodes[kind]; ok {
		return code
	}
	return exitCodes[KindInternal]
}

// Error is the error value that carries a contract kind up to main, through any
// number of wrapping layers. Only this type is recognised by From.
type Error struct {
	Kind    Kind
	Message string
	Hint    string
}

// Error returns the message: it is the text shown to the user in plain form.
func (e *Error) Error() string {
	return e.Message
}

// New builds an error of the given kind. The message is user-facing Spanish
// text (arranque §6.4, decision 6); hint is optional guidance for the caller
// and may be empty. The kind is normalised when the envelope is built (From),
// so an unknown kind can never reach the wire with an invented code.
func New(kind Kind, message, hint string) *Error {
	return &Error{Kind: kind, Message: message, Hint: hint}
}

// Envelope is the wire form of an error: {"error": {"type","message",
// "exit_code","hint"}}, with hint omitted when empty (arranque §6.4).
type Envelope struct {
	Error Body `json:"error"`
}

// Body is the inner object of the envelope.
type Body struct {
	Type     string `json:"type"`
	Message  string `json:"message"`
	ExitCode int    `json:"exit_code"`
	Hint     string `json:"hint,omitempty"`
}

// From recognises our own errors, even wrapped by intermediate layers, and
// returns their envelope. Any other error yields (zero Envelope, false): the
// caller decides what to do with it (main treats that as internal).
func From(err error) (Envelope, bool) {
	var e *Error
	if !errors.As(err, &e) {
		return Envelope{}, false
	}
	return envelope(e.Kind, e.Message, e.Hint), true
}

// envelope builds the wire form. An unknown kind falls into internal with code
// 1 and says so in the message: the original kind is not echoed and no new
// code is invented (arranque §6.4, decision 4).
func envelope(kind Kind, message, hint string) Envelope {
	code, ok := exitCodes[kind]
	if !ok {
		code = exitCodes[KindInternal]
		message = fmt.Sprintf("tipo de error desconocido %q; tratado como %q con código 1. %s",
			string(kind), KindInternal, message)
		kind = KindInternal
	}
	return Envelope{
		Error: Body{
			Type:     string(kind),
			Message:  message,
			ExitCode: code,
			Hint:     hint,
		},
	}
}
