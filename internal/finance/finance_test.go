package finance

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shopspring/decimal"
)

// Los casos de referencia viven en ../../testdata/finance y son la fuente de
// verdad: este test nunca escribe los números esperados a mano (arranque §9).
var testdataDir = filepath.Join("..", "..", "testdata", "finance")

type refRow struct {
	Month            int    `json:"month"`
	Payment          string `json:"payment"`
	Interest         string `json:"interest"`
	PrincipalRepaid  string `json:"principal_repaid"`
	OutstandingAfter string `json:"outstanding_after"`
}

type refCase struct {
	ID     string `json:"id"`
	Inputs struct {
		Principal     string `json:"principal"`
		AnnualRatePct string `json:"annual_rate_pct"`
		TermMonths    int    `json:"term_months"`
	} `json:"inputs"`
	Expected struct {
		Payment            string `json:"payment"`
		TotalInterest      string `json:"total_interest"`
		TotalPaid          string `json:"total_paid"`
		FirstRow           refRow `json:"first_row"`
		LastRow            refRow `json:"last_row"`
		SumPrincipalRepaid string `json:"sum_principal_repaid"`
		FinalOutstanding   string `json:"final_outstanding"`
	} `json:"expected"`
}

func mustDecimal(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("decimal.NewFromString(%q): %v", s, err)
	}
	return d
}

func loadReferenceCases(t *testing.T, pattern string, want int) []refCase {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(testdataDir, pattern))
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	if len(paths) != want {
		t.Fatalf("se esperaban %d casos de referencia con el patrón %q, hay %d: %v", want, pattern, len(paths), paths)
	}
	cases := make([]refCase, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("leer %s: %v", path, err)
		}
		var c refCase
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatalf("decodificar %s: %v", path, err)
		}
		cases = append(cases, c)
	}
	return cases
}

func checkRow(t *testing.T, label string, got Row, want refRow) {
	t.Helper()
	if got.Month != want.Month {
		t.Errorf("%s: mes = %d, se esperaba %d", label, got.Month, want.Month)
	}
	fields := []struct {
		name string
		got  decimal.Decimal
		want string
	}{
		{"payment", got.Payment, want.Payment},
		{"interest", got.Interest, want.Interest},
		{"principal", got.Principal, want.PrincipalRepaid},
		{"balance", got.Balance, want.OutstandingAfter},
	}
	for _, f := range fields {
		if !f.got.Equal(mustDecimal(t, f.want)) {
			t.Errorf("%s: %s = %s, se esperaba %s", label, f.name, f.got.String(), f.want)
		}
	}
}

// TestFixedReferenceCases valida Payment y Amortization contra los cinco casos
// fijos de testdata/finance. Compara con decimal.Equal, nunca por formato.
func TestFixedReferenceCases(t *testing.T) {
	for _, tc := range loadReferenceCases(t, "case-0[1-5]-*.json", 5) {
		t.Run(tc.ID, func(t *testing.T) {
			principal := mustDecimal(t, tc.Inputs.Principal)
			rate := mustDecimal(t, tc.Inputs.AnnualRatePct)
			months := tc.Inputs.TermMonths

			payment, err := Payment(principal, rate, months)
			if err != nil {
				t.Fatalf("Payment: %v", err)
			}
			if !payment.Equal(mustDecimal(t, tc.Expected.Payment)) {
				t.Errorf("Payment = %s, se esperaba %s", payment.String(), tc.Expected.Payment)
			}

			sched, err := Amortization(Loan{Principal: principal, AnnualRate: rate, Months: months})
			if err != nil {
				t.Fatalf("Amortization: %v", err)
			}
			if len(sched.Rows) != months {
				t.Fatalf("filas = %d, se esperaban %d", len(sched.Rows), months)
			}

			checkRow(t, "primera fila", sched.Rows[0], tc.Expected.FirstRow)
			checkRow(t, "última fila", sched.Rows[len(sched.Rows)-1], tc.Expected.LastRow)

			if !sched.TotalInterest.Equal(mustDecimal(t, tc.Expected.TotalInterest)) {
				t.Errorf("TotalInterest = %s, se esperaba %s", sched.TotalInterest.String(), tc.Expected.TotalInterest)
			}
			if !sched.TotalPaid.Equal(mustDecimal(t, tc.Expected.TotalPaid)) {
				t.Errorf("TotalPaid = %s, se esperaba %s", sched.TotalPaid.String(), tc.Expected.TotalPaid)
			}
			if !sched.Rows[len(sched.Rows)-1].Balance.Equal(mustDecimal(t, tc.Expected.FinalOutstanding)) {
				t.Errorf("capital final = %s, se esperaba %s",
					sched.Rows[len(sched.Rows)-1].Balance.String(), tc.Expected.FinalOutstanding)
			}

			sumPrincipal := decimal.Zero
			for _, r := range sched.Rows {
				sumPrincipal = sumPrincipal.Add(r.Principal)
			}
			if !sumPrincipal.Equal(mustDecimal(t, tc.Expected.SumPrincipalRepaid)) {
				t.Errorf("suma de amortizaciones = %s, se esperaba %s", sumPrincipal.String(), tc.Expected.SumPrincipalRepaid)
			}
		})
	}
}

// TestCase02ZeroRate cierra exactamente con i = 0: cuota = capital / meses,
// interés cero en todas las filas, sin división por cero.
func TestCase02ZeroRate(t *testing.T) {
	cases := loadReferenceCases(t, "case-02-*.json", 1)
	tc := cases[0]
	principal := mustDecimal(t, tc.Inputs.Principal)
	rate := mustDecimal(t, tc.Inputs.AnnualRatePct)
	months := tc.Inputs.TermMonths

	payment, err := Payment(principal, rate, months)
	if err != nil {
		t.Fatalf("Payment con i = 0: %v", err)
	}
	if !payment.Equal(mustDecimal(t, tc.Expected.Payment)) {
		t.Errorf("Payment = %s, se esperaba %s", payment.String(), tc.Expected.Payment)
	}

	sched, err := Amortization(Loan{Principal: principal, AnnualRate: rate, Months: months})
	if err != nil {
		t.Fatalf("Amortization con i = 0: %v", err)
	}
	if !sched.TotalInterest.IsZero() {
		t.Errorf("TotalInterest = %s, se esperaba 0 exacto", sched.TotalInterest.String())
	}
	for _, r := range sched.Rows {
		if !r.Interest.IsZero() {
			t.Fatalf("fila %d: interés = %s, se esperaba 0 exacto", r.Month, r.Interest.String())
		}
		if !r.Payment.Equal(payment) {
			t.Errorf("fila %d: cuota = %s, se esperaba %s constante con i = 0", r.Month, r.Payment.String(), payment.String())
		}
	}
	last := sched.Rows[len(sched.Rows)-1]
	if !last.Balance.Equal(mustDecimal(t, tc.Expected.FinalOutstanding)) {
		t.Errorf("capital final = %s, se esperaba %s", last.Balance.String(), tc.Expected.FinalOutstanding)
	}
}

// TestPropertiesOverFixedCases comprueba propiedades sobre los cinco casos
// fijos: la suma de amortizaciones devuelve el capital, el capital pendiente
// nunca crece ni se vuelve negativo, y TotalPaid == TotalInterest + capital.
func TestPropertiesOverFixedCases(t *testing.T) {
	for _, tc := range loadReferenceCases(t, "case-0[1-5]-*.json", 5) {
		t.Run(tc.ID, func(t *testing.T) {
			loan := Loan{
				Principal:  mustDecimal(t, tc.Inputs.Principal),
				AnnualRate: mustDecimal(t, tc.Inputs.AnnualRatePct),
				Months:     tc.Inputs.TermMonths,
			}
			sched, err := Amortization(loan)
			if err != nil {
				t.Fatalf("Amortization: %v", err)
			}

			sumPrincipal := decimal.Zero
			prevBalance := loan.Principal
			for _, r := range sched.Rows {
				sumPrincipal = sumPrincipal.Add(r.Principal)
				if r.Balance.Sign() < 0 {
					t.Errorf("fila %d: capital pendiente negativo: %s", r.Month, r.Balance.String())
				}
				if r.Balance.GreaterThan(prevBalance) {
					t.Errorf("fila %d: el capital crece: %s > %s", r.Month, r.Balance.String(), prevBalance.String())
				}
				prevBalance = r.Balance
			}
			if !sumPrincipal.Equal(loan.Principal) {
				t.Errorf("suma de amortizaciones = %s, se esperaba el capital %s", sumPrincipal.String(), loan.Principal.String())
			}
			if !sched.TotalPaid.Equal(sched.TotalInterest.Add(loan.Principal)) {
				t.Errorf("TotalPaid %s != TotalInterest %s + capital %s",
					sched.TotalPaid.String(), sched.TotalInterest.String(), loan.Principal.String())
			}
		})
	}
}

// TestInputValidation comprueba los errores centinela con errors.Is.
func TestInputValidation(t *testing.T) {
	principal := decimal.NewFromInt(10000)
	rate := decimal.NewFromInt(3)
	negative := decimal.NewFromInt(-1)

	t.Run("Payment", func(t *testing.T) {
		if _, err := Payment(negative, rate, 12); !errors.Is(err, ErrNegativePrincipal) {
			t.Errorf("capital negativo: err = %v, se esperaba ErrNegativePrincipal", err)
		}
		if _, err := Payment(principal, rate, -12); !errors.Is(err, ErrNegativeMonths) {
			t.Errorf("meses negativos: err = %v, se esperaba ErrNegativeMonths", err)
		}
		if _, err := Payment(principal, rate, 0); !errors.Is(err, ErrZeroMonths) {
			t.Errorf("meses cero: err = %v, se esperaba ErrZeroMonths", err)
		}
		if _, err := Payment(principal, negative, 12); !errors.Is(err, ErrNegativeRate) {
			t.Errorf("tipo negativo: err = %v, se esperaba ErrNegativeRate", err)
		}
	})

	t.Run("Amortization", func(t *testing.T) {
		if _, err := Amortization(Loan{Principal: negative, AnnualRate: rate, Months: 12}); !errors.Is(err, ErrNegativePrincipal) {
			t.Errorf("capital negativo: err = %v, se esperaba ErrNegativePrincipal", err)
		}
		if _, err := Amortization(Loan{Principal: principal, AnnualRate: rate, Months: -12}); !errors.Is(err, ErrNegativeMonths) {
			t.Errorf("meses negativos: err = %v, se esperaba ErrNegativeMonths", err)
		}
		if _, err := Amortization(Loan{Principal: principal, AnnualRate: rate, Months: 0}); !errors.Is(err, ErrZeroMonths) {
			t.Errorf("meses cero: err = %v, se esperaba ErrZeroMonths", err)
		}
		if _, err := Amortization(Loan{Principal: principal, AnnualRate: negative, Months: 12}); !errors.Is(err, ErrNegativeRate) {
			t.Errorf("tipo negativo: err = %v, se esperaba ErrNegativeRate", err)
		}
	})
}
