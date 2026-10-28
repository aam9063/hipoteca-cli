// Package finance is pure: it receives data and returns results. No network, no file
// access, no clock (time is injected). Every amount and rate is shopspring/decimal,
// never float64: a float64 in this package is a defect (P-07).
package finance
