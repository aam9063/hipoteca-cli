package loan

import (
	"errors"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// completa devuelve el préstamo del ejemplo de docs/ARRANQUE.md §4.1, con
// todos los campos de la escritura explícitos: es el préstamo de referencia
// para los tests de validación.
func completa() Loan {
	initialPeriod := 12
	offset := 2
	sinSuelo := ""
	return Loan{
		Name:                  "Casa",
		Type:                  TypeVariable,
		StartDate:             "2021-06-01",
		OriginalPrincipal:     "180000.00",
		TermMonths:            360,
		PaymentDay:            1,
		InitialRate:           "1.25",
		InitialPeriodMonths:   &initialPeriod,
		Index:                 "euribor-1y",
		Spread:                "0.89",
		SpreadUnbonified:      "1.39",
		RevisionEveryMonths:   12,
		ReferenceOffsetMonths: &offset,
		FloorRate:             &sinSuelo,
		RateRounding:          "none",
		InterestConvention:    InterestConventionMonthlyNominal,
		PaymentRounding:       PaymentRoundingHalfUpCents,
		EarlyRepaymentFeePct:  "0",
	}
}

// TestPrestamoCompletoValida: el préstamo del ejemplo del arranque, con la
// versión "sin suelo" explícita (floor_rate = "") y con suelo, valida sin
// error y sin campos ausentes.
func TestPrestamoCompletoValida(t *testing.T) {
	suelo := "2.00"
	conSuelo := completa()
	conSuelo.FloorRate = &suelo
	for name, l := range map[string]Loan{
		"sin suelo explícito": completa(),
		"con suelo":           conSuelo,
	} {
		t.Run(name, func(t *testing.T) {
			if err := l.Validate(); err != nil {
				t.Fatalf("Validate() del préstamo completo: error = %v", err)
			}
			if missing := l.MissingFields(); len(missing) != 0 {
				t.Fatalf("MissingFields() = %v, want vacío", missing)
			}
		})
	}
}

// TestCampoObligatorioAusente: cada campo obligatorio ausente produce el
// centinela IncompleteError nombrando el campo (LOAN-01.1). Los datos que
// dependen de la escritura no se suponen nunca.
func TestCampoObligatorioAusente(t *testing.T) {
	cases := []struct {
		name  string
		rozar func(*Loan)
		campo string
	}{
		{"name", func(l *Loan) { l.Name = "" }, "name"},
		{"type", func(l *Loan) { l.Type = "" }, "type"},
		{"start_date", func(l *Loan) { l.StartDate = "" }, "start_date"},
		{"original_principal", func(l *Loan) { l.OriginalPrincipal = "" }, "original_principal"},
		{"term_months", func(l *Loan) { l.TermMonths = 0 }, "term_months"},
		{"payment_day", func(l *Loan) { l.PaymentDay = 0 }, "payment_day"},
		{"initial_rate", func(l *Loan) { l.InitialRate = "" }, "initial_rate"},
		{"initial_period_months", func(l *Loan) { l.InitialPeriodMonths = nil }, "initial_period_months"},
		{"index", func(l *Loan) { l.Index = "" }, "index"},
		{"spread", func(l *Loan) { l.Spread = "" }, "spread"},
		{"revision_every_months", func(l *Loan) { l.RevisionEveryMonths = 0 }, "revision_every_months"},
		{"reference_offset_months", func(l *Loan) { l.ReferenceOffsetMonths = nil }, "reference_offset_months"},
		{"floor_rate", func(l *Loan) { l.FloorRate = nil }, "floor_rate"},
		{"rate_rounding", func(l *Loan) { l.RateRounding = "" }, "rate_rounding"},
		{"interest_convention", func(l *Loan) { l.InterestConvention = "" }, "interest_convention"},
		{"payment_rounding", func(l *Loan) { l.PaymentRounding = "" }, "payment_rounding"},
		{"early_repayment_fee_pct", func(l *Loan) { l.EarlyRepaymentFeePct = "" }, "early_repayment_fee_pct"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := completa()
			tc.rozar(&l)
			err := l.Validate()
			var inc *IncompleteError
			if !errors.As(err, &inc) {
				t.Fatalf("Validate() = %v, want IncompleteError", err)
			}
			if inc.Field != tc.campo {
				t.Fatalf("el centinela nombra el campo %q, want %q", inc.Field, tc.campo)
			}
			if !strings.Contains(err.Error(), tc.campo) {
				t.Fatalf("el mensaje %q no nombra el campo %q", err.Error(), tc.campo)
			}
			// El campo también aparece en MissingFields, que es lo que
			// muestra `loan show` (arranque §4.1: show avisa si faltan).
			missing := l.MissingFields()
			if len(missing) != 1 || missing[0] != tc.campo {
				t.Fatalf("MissingFields() = %v, want [%s]", missing, tc.campo)
			}
		})
	}
}

// TestSyncIncompleto: un sync presente pero con un campo vacío es un préstamo
// incompleto y el campo se nombra con su ruta completa (sync.<campo>).
func TestSyncIncompleto(t *testing.T) {
	cases := []struct {
		name  string
		rozar func(*Sync)
		campo string
	}{
		{"date", func(s *Sync) { s.Date = "" }, "sync.date"},
		{"outstanding", func(s *Sync) { s.OutstandingPrincipal = "" }, "sync.outstanding_principal"},
		{"rate", func(s *Sync) { s.CurrentRate = "" }, "sync.current_rate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := completa()
			l.Sync = &Sync{Date: "2026-10-01", OutstandingPrincipal: "152341.18", CurrentRate: "3.12"}
			tc.rozar(l.Sync)
			err := l.Validate()
			var inc *IncompleteError
			if !errors.As(err, &inc) || inc.Field != tc.campo {
				t.Fatalf("Validate() = %v, want IncompleteError con campo %q", err, tc.campo)
			}
		})
	}
}

// TestSyncCompletoValida: un sync con los tres campos no invalida el préstamo.
func TestSyncCompletoValida(t *testing.T) {
	l := completa()
	l.Sync = &Sync{Date: "2026-10-01", OutstandingPrincipal: "152341.18", CurrentRate: "3.12"}
	if err := l.Validate(); err != nil {
		t.Fatalf("Validate() con sync completo: error = %v", err)
	}
}

// TestValoresInvalidos: los valores presentes pero imposibles son errores de
// uso (2), nunca centinela de incompleto ni error interno (1).
func TestValoresInvalidos(t *testing.T) {
	cases := []struct {
		name  string
		rozar func(*Loan)
	}{
		{"type desconocido", func(l *Loan) { l.Type = "revolving" }},
		{"start_date no es fecha", func(l *Loan) { l.StartDate = "junio" }},
		{"original_principal no es decimal", func(l *Loan) { l.OriginalPrincipal = "mil euros" }},
		{"original_principal negativo", func(l *Loan) { l.OriginalPrincipal = "-1" }},
		{"original_principal cero", func(l *Loan) { l.OriginalPrincipal = "0" }},
		{"term_months negativo", func(l *Loan) { l.TermMonths = -5 }},
		{"payment_day fuera de rango", func(l *Loan) { l.PaymentDay = 32 }},
		{"initial_rate no es decimal", func(l *Loan) { l.InitialRate = "x" }},
		{"initial_rate negativo", func(l *Loan) { l.InitialRate = "-0.1" }},
		{"initial_period_months negativo", func(l *Loan) {
			cero := -1
			l.InitialPeriodMonths = &cero
		}},
		{"initial_period_months supera el plazo", func(l *Loan) {
			muchos := 361
			l.InitialPeriodMonths = &muchos
		}},
		{"spread no es decimal", func(l *Loan) { l.Spread = "x" }},
		{"spread_unbonified no es decimal", func(l *Loan) { l.SpreadUnbonified = "x" }},
		{"revision_every_months negativo", func(l *Loan) { l.RevisionEveryMonths = -12 }},
		{"reference_offset_months negativo", func(l *Loan) {
			menos := -1
			l.ReferenceOffsetMonths = &menos
		}},
		{"floor_rate no es decimal", func(l *Loan) {
			x := "x"
			l.FloorRate = &x
		}},
		{"floor_rate negativo", func(l *Loan) {
			menos := "-1.00"
			l.FloorRate = &menos
		}},
		{"rate_rounding no soportado", func(l *Loan) { l.RateRounding = "1/3" }},
		{"interest_convention no soportada", func(l *Loan) { l.InterestConvention = "daily-actual" }},
		{"payment_rounding no soportado", func(l *Loan) { l.PaymentRounding = "truncar" }},
		{"early_repayment_fee_pct no es decimal", func(l *Loan) { l.EarlyRepaymentFeePct = "x" }},
		{"early_repayment_fee_pct negativo", func(l *Loan) { l.EarlyRepaymentFeePct = "-0.5" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := completa()
			tc.rozar(&l)
			err := l.Validate()
			var e *errs.Error
			if !errors.As(err, &e) {
				t.Fatalf("Validate() = %v (%T), want error de uso del contrato", err, err)
			}
			if e.Kind != errs.KindUsage {
				t.Fatalf("kind = %q, want usage", e.Kind)
			}
			var inc *IncompleteError
			if errors.As(err, &inc) {
				t.Fatalf("un valor inválido no es un préstamo incompleto: %v", err)
			}
		})
	}
}

// TestRateRoundingsSoportados: los cuatro redondeos de finance (convención 1.5
// de docs/calculos.md) pasan la validación.
func TestRateRoundingsSoportados(t *testing.T) {
	for _, r := range []string{"none", "1/8", "1/4", "0.05"} {
		l := completa()
		l.RateRounding = r
		if err := l.Validate(); err != nil {
			t.Fatalf("Validate() con rate_rounding %q: error = %v", r, err)
		}
	}
}
