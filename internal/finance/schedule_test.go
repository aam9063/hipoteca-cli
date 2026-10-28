package finance

import (
	"testing"

	"github.com/shopspring/decimal"
)

// mustAmortization ejecuta Amortization y aborta el test si falla.
func mustAmortization(t *testing.T, loan Loan) Schedule {
	t.Helper()
	sched, err := Amortization(loan)
	if err != nil {
		t.Fatalf("Amortization(%s, %s, %d): %v", loan.Principal.String(), loan.AnnualRate.String(), loan.Months, err)
	}
	return sched
}

// TestScheduleGridProperties recorre una rejilla de capitales × tipos × plazos
// y comprueba las invariantes del cuadro en cada combinación:
//
//   - ninguna cuota negativa;
//   - ningún interés negativo (el capital nunca es negativo, así que el
//     interés redondeado half-up tampoco puede serlo);
//   - ningún capital pendiente negativo (FIN-01.4);
//   - la suma de amortizaciones devuelve el capital exactamente;
//   - la última fila deja el capital en exactamente 0.00;
//   - el cuadro no tiene más filas que meses.
//
// La zona extrema (tipos de dos dígitos con plazos largos) es la que destapó
// el defecto FIN-01: el excedente de redondeo half-up de la cuota liquidaba el
// préstamo antes del mes Months y el bucle seguía emitiendo filas fantasma con
// cuota y capital negativos. La rejilla es determinista y tarda < 2 s.
func TestScheduleGridProperties(t *testing.T) {
	principals := []string{"0.01", "10000", "150000", "183450.75", "999999.99"}
	rates := []string{"0.01", "1.50", "3.10", "6.25", "12.00", "23.62", "25.00"}
	terms := []int{1, 12, 60, 120, 240, 360, 480}

	for _, p := range principals {
		for _, r := range rates {
			for _, n := range terms {
				loan := Loan{
					Principal:  mustDecimal(t, p),
					AnnualRate: mustDecimal(t, r),
					Months:     n,
				}
				label := p + "·" + r + "%·" + itoa(n)
				t.Run(label, func(t *testing.T) {
					sched := mustAmortization(t, loan)

					if len(sched.Rows) < 1 || len(sched.Rows) > loan.Months {
						t.Fatalf("filas = %d, se esperaba entre 1 y %d", len(sched.Rows), loan.Months)
					}

					sumPrincipal := decimal.Zero
					prevBalance := loan.Principal
					for _, row := range sched.Rows {
						if row.Payment.Sign() < 0 {
							t.Errorf("fila %d: cuota negativa: %s", row.Month, row.Payment.String())
						}
						if row.Interest.Sign() < 0 {
							t.Errorf("fila %d: interés negativo: %s", row.Month, row.Interest.String())
						}
						if row.Balance.Sign() < 0 {
							t.Errorf("fila %d: capital pendiente negativo: %s", row.Month, row.Balance.String())
						}
						if row.Balance.GreaterThan(prevBalance) {
							t.Errorf("fila %d: el capital crece: %s > %s", row.Month, row.Balance.String(), prevBalance.String())
						}
						if row.Balance.IsZero() && row.Month != sched.Rows[len(sched.Rows)-1].Month {
							t.Errorf("fila %d: capital 0 antes de la última fila (fila fantasma)", row.Month)
						}
						prevBalance = row.Balance
						sumPrincipal = sumPrincipal.Add(row.Principal)
					}

					if !sumPrincipal.Equal(loan.Principal) {
						t.Errorf("suma de amortizaciones = %s, se esperaba el capital %s",
							sumPrincipal.String(), loan.Principal.String())
					}
					if !sched.Rows[len(sched.Rows)-1].Balance.IsZero() {
						t.Errorf("capital final = %s, se esperaba 0 exacto",
							sched.Rows[len(sched.Rows)-1].Balance.String())
					}
					if !sched.TotalPaid.Equal(sched.TotalInterest.Add(loan.Principal)) {
						t.Errorf("TotalPaid %s != TotalInterest %s + capital %s",
							sched.TotalPaid.String(), sched.TotalInterest.String(), loan.Principal.String())
					}
				})
			}
		}
	}
}

// TestReproducedCaseNoPhantomRows es la regresión del caso reproducido por el
// verificador independiente: 150000.00 al 23.62 % a 480 meses. Antes del
// arreglo el mes 479 pagaba 2952.76 sobre un capital que sólo soportaba
// 2743.77, el capital llegaba a −264.99 y el mes 480 emitía una cuota
// negativa (−270.20, interés −5.21), con suma de amortizaciones 149735.01.
//
// Con el arreglo el préstamo se cierra antes del mes 480: la mensualidad en
// la que la cuota redondeada superaría capital + interés pasa a ser la última
// (paga exactamente capital + interés) y el bucle se detiene. Este cuadro
// tiene 479 filas, no 480: la cuota redondeada supera capital + interés por
// primera vez en el mes 479, que paga exactamente 2687.77 y cierra el préstamo.
func TestReproducedCaseNoPhantomRows(t *testing.T) {
	loan := Loan{
		Principal:  mustDecimal(t, "150000.00"),
		AnnualRate: mustDecimal(t, "23.62"),
		Months:     480,
	}
	sched := mustAmortization(t, loan)

	if len(sched.Rows) > loan.Months {
		t.Fatalf("filas = %d, se esperaban %d o menos", len(sched.Rows), loan.Months)
	}

	sumPrincipal := decimal.Zero
	for _, row := range sched.Rows {
		if row.Payment.Sign() < 0 {
			t.Errorf("fila %d: cuota negativa: %s", row.Month, row.Payment.String())
		}
		if row.Balance.Sign() < 0 {
			t.Errorf("fila %d: capital pendiente negativo: %s", row.Month, row.Balance.String())
		}
		sumPrincipal = sumPrincipal.Add(row.Principal)
	}
	if !sumPrincipal.Equal(loan.Principal) {
		t.Errorf("suma de amortizaciones = %s, se esperaba 150000.00", sumPrincipal.String())
	}
	if !sched.Rows[len(sched.Rows)-1].Balance.IsZero() {
		t.Errorf("capital final = %s, se esperaba 0 exacto",
			sched.Rows[len(sched.Rows)-1].Balance.String())
	}
}

// TestNegativeRateSchedule: un préstamo sin suelo con Euríbor negativo
// (índice −0.5 % + diferencial 0.2 % = TIN −0.3 %, decisión 3 de FIN-02) debe
// poder amortizarse: interés mensual negativo, amortización mayor que la
// cuota, cierre exacto a 0.00 y suma de amortizaciones igual al capital. La
// rejilla cubre plazos cortos y largos con tipos negativos suaves.
func TestNegativeRateSchedule(t *testing.T) {
	rates := []string{"-0.01", "-0.3", "-1.00"}
	terms := []int{12, 240, 360}
	for _, r := range rates {
		for _, n := range terms {
			loan := Loan{
				Principal:  mustDecimal(t, "100000"),
				AnnualRate: mustDecimal(t, r),
				Months:     n,
			}
			label := r + "%·" + itoa(n)
			t.Run(label, func(t *testing.T) {
				sched := mustAmortization(t, loan)

				if len(sched.Rows) < 1 || len(sched.Rows) > loan.Months {
					t.Fatalf("filas = %d, se esperaba entre 1 y %d", len(sched.Rows), loan.Months)
				}

				payment, err := Payment(loan.Principal, loan.AnnualRate, loan.Months)
				if err != nil {
					t.Fatalf("Payment con tipo negativo: %v", err)
				}
				perMonth := loan.Principal.Div(mustDecimal(t, itoa(loan.Months)))
				if !payment.LessThan(perMonth) {
					t.Errorf("cuota %s no es menor que capital/meses %s", payment.String(), perMonth.String())
				}

				sumPrincipal := decimal.Zero
				prevBalance := loan.Principal
				first := true
				for _, row := range sched.Rows {
					if row.Payment.Sign() <= 0 {
						t.Errorf("fila %d: cuota no positiva: %s", row.Month, row.Payment.String())
					}
					// El interés es negativo mientras el capital soporta
					// céntimos; en las últimas filas |i|·capital puede
					// redondear a 0.00 exacto, que también es válido.
					if row.Interest.Sign() > 0 {
						t.Errorf("fila %d: interés positivo con tipo negativo: %s", row.Month, row.Interest.String())
					}
					if first && row.Interest.Sign() >= 0 {
						t.Errorf("primera fila: interés = %s, se esperaba negativo", row.Interest.String())
					}
					first = false
					if row.Principal.Sign() <= 0 {
						t.Errorf("fila %d: amortización no positiva: %s", row.Month, row.Principal.String())
					}
					if row.Balance.Sign() < 0 || row.Balance.GreaterThan(prevBalance) {
						t.Errorf("fila %d: capital pendiente inválido: %s (anterior %s)",
							row.Month, row.Balance.String(), prevBalance.String())
					}
					prevBalance = row.Balance
					sumPrincipal = sumPrincipal.Add(row.Principal)
				}
				if !sumPrincipal.Equal(loan.Principal) {
					t.Errorf("suma de amortizaciones = %s, se esperaba el capital %s",
						sumPrincipal.String(), loan.Principal.String())
					t.Errorf("última fila: capital = %s, se esperaba 0.00 exacto",
						sched.Rows[len(sched.Rows)-1].Balance.String())
				}
				if sched.TotalInterest.Sign() >= 0 {
					t.Errorf("interés total = %s, se esperaba negativo", sched.TotalInterest.String())
				}
				if !sched.TotalPaid.Equal(sched.TotalInterest.Add(loan.Principal)) {
					t.Errorf("TotalPaid %s != TotalInterest %s + capital %s",
						sched.TotalPaid.String(), sched.TotalInterest.String(), loan.Principal.String())
				}
			})
		}
	}
}

// TestCompoundScheduleNegativeRate: NewRate con índice negativo y sin suelo
// devuelve un TIN negativo (decisión 3) y CompoundSchedule construye su cuadro
// con el mismo núcleo, cerrando a 0.00 exacto.
func TestCompoundScheduleNegativeRate(t *testing.T) {
	rate, err := NewRate(mustDecimal(t, "-0.5"), mustDecimal(t, "0.2"), nil, RateRoundingNone)
	if err != nil {
		t.Fatalf("NewRate(-0.5, 0.2, nil, none): %v", err)
	}
	if rate.Final.Sign() >= 0 {
		t.Fatalf("TIN final = %s, se esperaba negativo", rate.Final.String())
	}

	loan := Loan{Principal: mustDecimal(t, "100000"), AnnualRate: rate.Final, Months: 12}
	sched, err := CompoundSchedule(loan, []Period{{Rate: rate.Final, Months: 12}})
	if err != nil {
		t.Fatalf("CompoundSchedule con TIN negativo: %v", err)
	}
	if len(sched.Rows) != 12 {
		t.Fatalf("filas = %d, se esperaban 12", len(sched.Rows))
	}
	if !sched.Rows[len(sched.Rows)-1].Balance.IsZero() {
		t.Errorf("capital final = %s, se esperaba 0 exacto",
			sched.Rows[len(sched.Rows)-1].Balance.String())
	}
	sumPrincipal := decimal.Zero
	for _, row := range sched.Rows {
		if row.Interest.Sign() >= 0 {
			t.Errorf("fila %d: interés no negativo: %s", row.Month, row.Interest.String())
		}
		sumPrincipal = sumPrincipal.Add(row.Principal)
	}
	if !sumPrincipal.Equal(loan.Principal) {
		t.Errorf("suma de amortizaciones = %s, se esperaba el capital %s",
			sumPrincipal.String(), loan.Principal.String())
	}
	if sched.TotalInterest.Sign() >= 0 {
		t.Errorf("interés total = %s, se esperaba negativo", sched.TotalInterest.String())
	}
}

// itoa evita importar strconv sólo para las etiquetas de la rejilla.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}
