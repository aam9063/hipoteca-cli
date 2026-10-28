// Package finance is pure: it receives data and returns results. No network, no file
// access, no clock (time is injected). Every amount and rate is shopspring/decimal,
// never float64: a float64 in this package is a defect (P-07).
//
// Negative annual rates are supported (FIN-02, decision 3): a loan without a floor and
// a negative index produces a negative TIN, amortized with negative monthly interest.
// Both rounding helpers are half-up away from zero, symmetric for both signs.
package finance
