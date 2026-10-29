package finance

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shopspring/decimal"
)

// Los casos 9 y 10 de ../../testdata/finance son la fuente de verdad: este
// test nunca escribe los números esperados a mano (arranque §9).
type prepayRefInputs struct {
	Mode                   string `json:"mode"`
	AfterMonth             int    `json:"after_month"`
	Amount                 string `json:"amount"`
	FeePct                 string `json:"fee_pct"`
	Fee                    string `json:"fee"`
	OutstandingBeforeEvent string `json:"outstanding_before_event"`
	OutstandingAfterEvent  string `json:"outstanding_after_event"`
	RemainingMonthsAtEvent int    `json:"remaining_months_at_event"`
	PaymentAfterEvent      string `json:"payment_after_event"`
	TermMonthsAfterEvent   int    `json:"term_months_after_event"`
	MonthsSaved            *int   `json:"months_saved"`
}

type prepayRefCase struct {
	ID     string `json:"id"`
	Inputs struct {
		Principal      string          `json:"principal"`
		AnnualRatePct  string          `json:"annual_rate_pct"`
		TermMonths     int             `json:"term_months"`
		EarlyRepayment prepayRefInputs `json:"early_repayment"`
	} `json:"inputs"`
	Expected struct {
		Payment                            string `json:"payment"`
		TotalInterest                      string `json:"total_interest"`
		LastRow                            refRow `json:"last_row"`
		SumPrincipalRepaidScheduleOnly     string `json:"sum_principal_repaid_schedule_only"`
		PaymentBeforeEvent                 string `json:"payment_before_event"`
		RowAtEventMonth                    refRow `json:"row_at_event_month"`
		FirstRowAfterEvent                 refRow `json:"first_row_after_event"`
		TotalInterestWithoutEarlyRepayment string `json:"total_interest_without_early_repayment"`
		InterestSaved                      string `json:"interest_saved"`
		MonthsSaved                        *int   `json:"months_saved"`
	} `json:"expected"`
}

func loadPrepayRefCase(t *testing.T, name string) prepayRefCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(testdataDir, name))
	if err != nil {
		t.Fatalf("leer %s: %v", name, err)
	}
	var tc prepayRefCase
	if err := json.Unmarshal(raw, &tc); err != nil {
		t.Fatalf("parsear %s: %v", name, err)
	}
	return tc
}

// mustPrepay ejecuta Prepay y aborta el test si falla.
func mustPrepay(t *testing.T, loan Loan, rep EarlyRepayment, mode Mode) Prepayment {
	t.Helper()
	pre, err := Prepay(loan, rep, mode)
	if err != nil {
		t.Fatalf("Prepay(%s, %s, tras mes %d, %s): %v",
			loan.Principal.String(), loan.AnnualRate.String(), rep.AfterMonth, string(mode), err)
	}
	return pre
}

// expectDecimal compara dos decimales por valor exacto, nunca por formato.
func expectDecimal(t *testing.T, got decimal.Decimal, want string, label string) {
	t.Helper()
	if !got.Equal(mustDecimal(t, want)) {
		t.Errorf("%s = %s, se esperaba %s", label, got.String(), want)
	}
}

// TestPrepayReferenceCases reproduce los casos 9 (reduce-payment) y 10
// (reduce-term) de testdata/finance al céntimo, leyendo siempre los números
// de los JSON (jamás escritos a mano aquí).
func TestPrepayReferenceCases(t *testing.T) {
	cases := []struct {
		file string
	}{
		{file: "case-09-amortiza-cuota.json"},
		{file: "case-10-amortiza-plazo.json"},
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			tc := loadPrepayRefCase(t, c.file)
			in := tc.Inputs.EarlyRepayment
			loan := Loan{
				Principal:  mustDecimal(t, tc.Inputs.Principal),
				AnnualRate: mustDecimal(t, tc.Inputs.AnnualRatePct),
				Months:     tc.Inputs.TermMonths,
			}
			rep := EarlyRepayment{
				Amount:     mustDecimal(t, in.Amount),
				FeePct:     mustDecimal(t, in.FeePct),
				AfterMonth: in.AfterMonth,
			}

			pre := mustPrepay(t, loan, rep, Mode(in.Mode))

			// Entradas y comisión: el desembolso es importe + comisión y el
			// capital baja exactamente por el importe (decisión 1).
			expectDecimal(t, pre.Amount, in.Amount, "Amount")
			expectDecimal(t, pre.FeePct, in.FeePct, "FeePct")
			expectDecimal(t, pre.Fee, in.Fee, "Fee")
			expectDecimal(t, pre.Outlay, mustDecimal(t, in.Amount).Add(mustDecimal(t, in.Fee)).String(), "Outlay")
			expectDecimal(t, pre.BalanceBefore, in.OutstandingBeforeEvent, "BalanceBefore")
			expectDecimal(t, pre.BalanceAfter, in.OutstandingAfterEvent, "BalanceAfter")

			// Cuota y plazo antes y después.
			expectDecimal(t, pre.PaymentBefore, tc.Expected.PaymentBeforeEvent, "PaymentBefore")
			expectDecimal(t, pre.PaymentAfter, in.PaymentAfterEvent, "PaymentAfter")
			if pre.TermBefore != in.AfterMonth+in.RemainingMonthsAtEvent {
				t.Errorf("TermBefore = %d, se esperaba %d", pre.TermBefore, in.AfterMonth+in.RemainingMonthsAtEvent)
			}
			if pre.TermAfter != in.TermMonthsAfterEvent {
				t.Errorf("TermAfter = %d, se esperaba %d", pre.TermAfter, in.TermMonthsAfterEvent)
			}
			wantMonthsSaved := 0
			if in.MonthsSaved != nil {
				wantMonthsSaved = *in.MonthsSaved
			}
			if pre.MonthsSaved != wantMonthsSaved {
				t.Errorf("MonthsSaved = %d, se esperaba %d", pre.MonthsSaved, wantMonthsSaved)
			}

			// Intereses: el ahorro se mide contra el mismo préstamo sin
			// amortizar (decisión 4).
			expectDecimal(t, pre.InterestBefore, tc.Expected.TotalInterestWithoutEarlyRepayment, "InterestBefore")
			expectDecimal(t, pre.InterestAfter, tc.Expected.TotalInterest, "InterestAfter")
			expectDecimal(t, pre.InterestSaved, tc.Expected.InterestSaved, "InterestSaved")

			// El cuadro resultante: filas del evento, primera posterior y
			// última, con cierre exacto a 0.00 (convención 1.3).
			sched := pre.Schedule
			if len(sched.Rows) != in.TermMonthsAfterEvent {
				t.Fatalf("filas del cuadro = %d, se esperaban %d", len(sched.Rows), in.TermMonthsAfterEvent)
			}
			wantEvent := tc.Expected.RowAtEventMonth
			gotEvent := sched.Rows[in.AfterMonth-1]
			if gotEvent.Month != wantEvent.Month || !gotEvent.Payment.Equal(mustDecimal(t, wantEvent.Payment)) ||
				!gotEvent.Interest.Equal(mustDecimal(t, wantEvent.Interest)) ||
				!gotEvent.Principal.Equal(mustDecimal(t, wantEvent.PrincipalRepaid)) ||
				!gotEvent.Balance.Equal(mustDecimal(t, wantEvent.OutstandingAfter)) {
				t.Errorf("fila del mes %d = %+v, se esperaba %+v", in.AfterMonth, gotEvent, wantEvent)
			}
			wantFirst := tc.Expected.FirstRowAfterEvent
			gotFirst := sched.Rows[in.AfterMonth]
			if gotFirst.Month != wantFirst.Month || !gotFirst.Payment.Equal(mustDecimal(t, wantFirst.Payment)) ||
				!gotFirst.Interest.Equal(mustDecimal(t, wantFirst.Interest)) ||
				!gotFirst.Principal.Equal(mustDecimal(t, wantFirst.PrincipalRepaid)) ||
				!gotFirst.Balance.Equal(mustDecimal(t, wantFirst.OutstandingAfter)) {
				t.Errorf("primera fila tras el evento = %+v, se esperaba %+v", gotFirst, wantFirst)
			}
			wantLast := tc.Expected.LastRow
			gotLast := sched.Rows[len(sched.Rows)-1]
			if gotLast.Month != wantLast.Month || !gotLast.Payment.Equal(mustDecimal(t, wantLast.Payment)) ||
				!gotLast.Interest.Equal(mustDecimal(t, wantLast.Interest)) ||
				!gotLast.Principal.Equal(mustDecimal(t, wantLast.PrincipalRepaid)) ||
				!gotLast.Balance.Equal(mustDecimal(t, wantLast.OutstandingAfter)) {
				t.Errorf("última fila = %+v, se esperaba %+v", gotLast, wantLast)
			}
			if !gotLast.Balance.IsZero() {
				t.Errorf("capital final = %s, se esperaba 0.00 exacto", gotLast.Balance.String())
			}

			// Decisión 1: la comisión (aquí 0 %) no entra en el cuadro; la
			// suma de amortizaciones del cuadro más el importe anticipado es
			// el capital original.
			sumPrincipal := decimal.Zero
			for _, row := range sched.Rows {
				sumPrincipal = sumPrincipal.Add(row.Principal)
			}
			expectDecimal(t, sumPrincipal, tc.Expected.SumPrincipalRepaidScheduleOnly, "suma de amortizaciones del cuadro")
		})
	}
}

// TestPrepayFeeCase: caso propio con comisión verificado con aritmética
// independiente, ajeno a los JSON de referencia.
//
// Préstamo del caso 1 (150000.00 al 3.10 % a 300 meses); amortización
// anticipada de 10000.00 tras la cuota 60 con comisión del 1 %.
//
// Derivación a mano:
//
//	comisión = 10000.00 · 1 / 100 = 100.00      (exacta, sin redondeo)
//	desembolso = 10000.00 + 100.00 = 10100.00
//	capital pendiente tras la cuota 60 = 128506.54 (caso 1)
//	capital tras la amortización = 128506.54 − 10000.00 = 118506.54
//
// Por la decisión 1 la comisión se paga aparte y NO reduce el capital: el
// saldo tras el evento es idéntico al de los casos 9 y 10, así que la cuota
// nueva (663.18), el interés restante (62312.38) y el ahorro bruto
// (65743.56 − 62312.38 = 3431.18) son exactamente los del caso 9. El beneficio
// neto para el usuario baja lo que cuesta la comisión, ni un céntimo más ni
// menos: 3431.18 − 100.00 = 3331.18. La comisión nunca puede AUMENTAR el
// ahorro: aquí el ahorro bruto es idéntico al caso sin comisión, y el neto es
// el bruto menos la comisión (el evento ocurre en el mismo mes, así que la
// comisión no compone dentro del cuadro).
//
// En reduce-term la misma lógica: plazo 275, 25 meses ahorrados y ahorro bruto
// 8004.38, idéntico al caso 10; neto 8004.38 − 100.00 = 7904.38.
func TestPrepayFeeCase(t *testing.T) {
	loan := Loan{
		Principal:  mustDecimal(t, "150000.00"),
		AnnualRate: mustDecimal(t, "3.10"),
		Months:     300,
	}
	rep := EarlyRepayment{
		Amount:     mustDecimal(t, "10000.00"),
		FeePct:     mustDecimal(t, "1"),
		AfterMonth: 60,
	}

	// reduce-payment: idéntico al caso 9 salvo la comisión y el desembolso.
	pre := mustPrepay(t, loan, rep, ReducePayment)
	expectDecimal(t, pre.Fee, "100.00", "Fee (1 % de 10000.00)")
	expectDecimal(t, pre.Outlay, "10100.00", "Outlay")
	expectDecimal(t, pre.BalanceBefore, "128506.54", "BalanceBefore")
	expectDecimal(t, pre.BalanceAfter, "118506.54", "BalanceAfter (decisión 1: la comisión no reduce capital)")
	expectDecimal(t, pre.PaymentAfter, "663.18", "PaymentAfter")
	expectDecimal(t, pre.InterestSaved, "3431.18", "InterestSaved (bruto, igual que el caso 9)")
	expectDecimal(t, pre.InterestSaved.Sub(pre.Fee), "3331.18", "ahorro neto = bruto − comisión")
	if pre.TermAfter != 300 || pre.MonthsSaved != 0 {
		t.Errorf("reduce-payment: TermAfter = %d, MonthsSaved = %d; se esperaban 300 y 0", pre.TermAfter, pre.MonthsSaved)
	}

	// reduce-term: idéntico al caso 10 salvo la comisión y el desembolso.
	pre = mustPrepay(t, loan, rep, ReduceTerm)
	expectDecimal(t, pre.Fee, "100.00", "Fee (1 % de 10000.00)")
	expectDecimal(t, pre.Outlay, "10100.00", "Outlay")
	expectDecimal(t, pre.PaymentAfter, "719.14", "PaymentAfter (cuota mantenida)")
	expectDecimal(t, pre.InterestSaved, "8004.38", "InterestSaved (bruto, igual que el caso 10)")
	expectDecimal(t, pre.InterestSaved.Sub(pre.Fee), "7904.38", "ahorro neto = bruto − comisión")
	if pre.TermAfter != 275 || pre.MonthsSaved != 25 {
		t.Errorf("reduce-term: TermAfter = %d, MonthsSaved = %d; se esperaban 275 y 25", pre.TermAfter, pre.MonthsSaved)
	}
	if last := pre.Schedule.Rows[len(pre.Schedule.Rows)-1]; !last.Balance.IsZero() {
		t.Errorf("reduce-term: capital final = %s, se esperaba 0.00 exacto", last.Balance.String())
	}

	// Redondeo half-up de la comisión: 100.00 · 1.005 % = 1.005 exacto, que
	// half-up lejos de cero da 1.01 (y no 1.00).
	pre = mustPrepay(t, loan, EarlyRepayment{
		Amount:     mustDecimal(t, "100.00"),
		FeePct:     mustDecimal(t, "1.005"),
		AfterMonth: 1,
	}, ReducePayment)
	expectDecimal(t, pre.Fee, "1.01", "Fee (100.00 · 1.005 % = 1.005 → half-up 1.01)")
	expectDecimal(t, pre.Outlay, "101.01", "Outlay")
}

// TestPrepayGridProperties es la propiedad de la decisión 5 (regla
// condicional según el signo del TIN): sobre una rejilla de capitales, tipos
// (incluidos el 0 % y un negativo suave), plazos, meses del evento, importes y
// comisiones de 0 a 3 %, en ambos modos:
//
//   - con TIN ≥ 0 el ahorro de intereses es siempre ≥ 0 (amortizar nunca
//     aumenta los intereses restantes);
//   - con TIN < 0 el ahorro es siempre ≤ 0 y su magnitud es exactamente el
//     subsidio no devengado: el interés a favor del prestatario que el cuadro
//     intacto ganaría en la misma ventana (y en los meses que vive de más),
//     verificado con la identidad exacta InterestSaved = InterestBeforeWindow
//     − InterestAfter + InterestBeforeTail, donde InterestBeforeTail es la
//     cola del cuadro intacto más allá de la ventana; la pérdida nunca excede
//     el subsidio íntegro de la vida intacta;
//   - ambas familias cierran en exactamente 0.00, mantienen cada fila sin
//     cuota, interés o capital negativos y preservan la identidad del capital.
//
// Las combinaciones en las que el propio núcleo ya no tiene cuadro
// (ErrUnpayableSchedule, sólo con importes o tipos irreales) se saltan: no hay
// préstamo que amortizar anticipadamente. Si alguna combinación válida viola
// la propiedad, el test falla: es un defecto del cálculo (decisión 5), nunca
// una razón para aflojar la aserción.
func TestPrepayGridProperties(t *testing.T) {
	principals := []string{"10000", "150000", "183450.75"}
	rates := []string{"0", "0.50", "3.10", "6.25", "-0.30"}
	terms := []int{60, 120, 300}
	amountFees := []string{"0", "0.25", "3"}
	modes := []Mode{ReducePayment, ReduceTerm}
	two := mustDecimal(t, "2")

	combo := 0
	for _, p := range principals {
		for _, r := range rates {
			for _, n := range terms {
				loan := Loan{Principal: mustDecimal(t, p), AnnualRate: mustDecimal(t, r), Months: n}
				baseline, err := Amortization(loan)
				if err != nil {
					if errors.Is(err, ErrUnpayableSchedule) {
						continue // combinación degenerada del núcleo, no hay cuadro
					}
					t.Fatalf("Amortization(%s, %s, %d): %v", p, r, n, err)
				}

				for _, afterMonth := range []int{1, len(baseline.Rows) / 2, len(baseline.Rows) - 1} {
					if afterMonth < 1 || afterMonth > len(baseline.Rows) {
						continue
					}
					balance := baseline.Rows[afterMonth-1].Balance
					amounts := []decimal.Decimal{mustDecimal(t, "100"), balance.Div(two), balance}
					for _, amount := range amounts {
						if amount.Sign() <= 0 || amount.GreaterThan(balance) {
							continue
						}
						for _, feePct := range amountFees {
							for _, mode := range modes {
								combo++
								label := p + "·" + r + "%·" + itoa(n) + "m·mes" + itoa(afterMonth) +
									"·" + amount.String() + "·fee" + feePct + "%·" + string(mode)
								rep := EarlyRepayment{
									Amount:     amount,
									FeePct:     mustDecimal(t, feePct),
									AfterMonth: afterMonth,
								}
								pre, err := Prepay(loan, rep, mode)
								if err != nil {
									t.Errorf("%s: Prepay: %v", label, err)
									continue
								}

								if !pre.BalanceAfter.Equal(pre.BalanceBefore.Sub(amount)) {
									t.Errorf("%s: el saldo baja %s, no exactamente el importe (decisión 1)",
										label, pre.BalanceBefore.Sub(pre.BalanceAfter).String())
								}
								if !pre.Outlay.Equal(amount.Add(pre.Fee)) {
									t.Errorf("%s: desembolso %s != importe %s + comisión %s",
										label, pre.Outlay.String(), amount.String(), pre.Fee.String())
								}
								if pre.Fee.Sign() < 0 {
									t.Errorf("%s: comisión negativa %s", label, pre.Fee.String())
								}

								// Decisión 5 (regla condicional según el signo del TIN).
								if loan.AnnualRate.Sign() >= 0 {
									if pre.InterestSaved.Sign() < 0 {
										t.Errorf("%s: ahorro negativo %s (intereses antes %s, después %s)",
											label, pre.InterestSaved.String(),
											pre.InterestBefore.String(), pre.InterestAfter.String())
									}
								} else {
									// TIN negativo: amortizar renuncia al subsidio que el
									// saldo devengaba a favor del prestatario. La pérdida
									// es exactamente ese subsidio no devengado.
									if pre.InterestSaved.Sign() > 0 {
										t.Errorf("%s: con TIN negativo el ahorro debe ser ≤ 0 y es %s", label, pre.InterestSaved.String())
									}
									prefix := decimal.Zero
									for _, row := range pre.Schedule.Rows[:afterMonth] {
										prefix = prefix.Add(row.Interest)
									}
									baseTail := pre.InterestBefore.Sub(prefix) // vida intacta tras el evento
									afterTail := pre.InterestAfter.Sub(prefix) // vida del cuadro nuevo
									if baseTail.Sign() > 0 || afterTail.Sign() > 0 || pre.InterestBeforeWindow.Sign() > 0 {
										t.Errorf("%s: subsidio con signo equivocado: vida intacta %s, vida nueva %s, ventana intacta %s",
											label, baseTail.String(), afterTail.String(), pre.InterestBeforeWindow.String())
									}
									if pre.InterestSaved.LessThan(baseTail) {
										t.Errorf("%s: la pérdida %s excede el subsidio íntegro de la vida intacta %s",
											label, pre.InterestSaved.String(), baseTail.String())
									}
									// Identidad exacta del subsidio no devengado:
									//   ahorro = ventana intacta − vida nueva + cola intacta
									//           más allá de la ventana (≤ 0)
									beyondWindow := baseTail.Sub(pre.InterestBeforeWindow)
									if beyondWindow.Sign() > 0 {
										t.Errorf("%s: la cola intacta más allá de la ventana gana subsidio %s", label, beyondWindow.String())
									}
									want := pre.InterestBeforeWindow.Sub(afterTail).Add(beyondWindow)
									if !pre.InterestSaved.Equal(want) {
										t.Errorf("%s: ahorro %s != subsidio no devengado %s (ventana %s, vida nueva %s, cola %s)",
											label, pre.InterestSaved.String(), want.String(),
											pre.InterestBeforeWindow.String(), afterTail.String(), beyondWindow.String())
									}
								}
								if pre.TermAfter > pre.TermBefore {
									t.Errorf("%s: el plazo se alarga: %d > %d", label, pre.TermAfter, pre.TermBefore)
								}
								if pre.MonthsSaved != pre.TermBefore-pre.TermAfter {
									t.Errorf("%s: MonthsSaved %d != TermBefore−TermAfter %d",
										label, pre.MonthsSaved, pre.TermBefore-pre.TermAfter)
								}

								// El préstamo cierra en exactamente 0.00: por la última
								// fila del cuadro o, si el importe agota el saldo, por el
								// propio evento (el cuadro termina en la fila del evento y
								// BalanceAfter es 0.00 exacto).
								lastBalance := pre.Schedule.Rows[len(pre.Schedule.Rows)-1].Balance
								closedByEvent := pre.BalanceAfter.IsZero()
								if closedByEvent {
									lastBalance = pre.BalanceAfter
								}
								if !lastBalance.IsZero() {
									t.Errorf("%s: capital final = %s, se esperaba 0.00 exacto",
										label, lastBalance.String())
								}
								// La suma de amortizaciones del cuadro más el importe
								// anticipado devuelve el capital original.
								sched := pre.Schedule
								sumPrincipal := decimal.Zero
								prevBalance := loan.Principal
								for _, row := range sched.Rows {
									if row.Payment.Sign() < 0 {
										t.Errorf("%s: fila %d: cuota negativa %s", label, row.Month, row.Payment.String())
									}
									if loan.AnnualRate.Sign() >= 0 && row.Interest.Sign() < 0 {
										t.Errorf("%s: fila %d: interés negativo %s", label, row.Month, row.Interest.String())
									}
									if row.Balance.Sign() < 0 || row.Balance.GreaterThan(prevBalance) {
										t.Errorf("%s: fila %d: capital pendiente inválido %s (anterior %s)",
											label, row.Month, row.Balance.String(), prevBalance.String())
									}
									prevBalance = row.Balance
									sumPrincipal = sumPrincipal.Add(row.Principal)
								}
								if !sumPrincipal.Add(amount).Equal(loan.Principal) {
									t.Errorf("%s: amortizaciones del cuadro %s + importe %s != capital %s",
										label, sumPrincipal.String(), amount.String(), loan.Principal.String())
								}
							}
						}
					}
				}
			}
		}
	}
	if combo == 0 {
		t.Fatal("la rejilla no ha ejecutado ninguna combinación")
	}
	t.Logf("combinaciones verificadas: %d", combo)
}

// TestPrepayNegativeRateSignedSaving: con TIN negativo (un Euríbor sin suelo
// de escritura, decisión 3 de FIN-02) el interés mensual es a favor del
// prestatario, así que amortizar renuncia al subsidio que el saldo devengaba:
// el ahorro se informa con signo negativo (decisión 5, regla condicional).
// // Préstamo 10000.00 al −0.30 % a 60 meses; amortización de 100.00 tras la
// cuota 1. Aritmética independiente:
//
//	interés del mes 1 = redondo_half_up(10000.00 · −0.30/100/12) = −2.50 (exacto)
//	interés total sin amortizar (núcleo, sujetado por la rejilla de FIN-01) = −76.07
//
// El prestatario recibe 76.07 de subsidio sin amortizar y 75.32 amortizando
// en reduce-payment: la pérdida es 76.07 − 75.32 = 0.75, y 1.46 en
// reduce-term (76.07 − 74.61), porque la cuota mantenida cierra antes y deja
// de devengar también la cola. El ahorro firmado ES esa pérdida; jamás se
// recorta a cero.
func TestPrepayNegativeRateSignedSaving(t *testing.T) {
	loan := Loan{Principal: mustDecimal(t, "10000.00"), AnnualRate: mustDecimal(t, "-0.30"), Months: 60}
	rep := EarlyRepayment{Amount: mustDecimal(t, "100.00"), AfterMonth: 1}

	// reduce-payment: mismo plazo, la pérdida es la diferencia de subsidio.
	pre := mustPrepay(t, loan, rep, ReducePayment)
	expectDecimal(t, pre.InterestBefore, "-76.07", "InterestBefore")
	expectDecimal(t, pre.InterestAfter, "-75.32", "InterestAfter")
	expectDecimal(t, pre.InterestSaved, "-0.75", "InterestSaved (subsidio renunciado, con signo)")
	// Ventana: ambos cuadros viven hasta el mes 60, así que el cuadro intacto
	// gana en la ventana (meses 2..60) justo su cola entera: −76.07 − (−2.50).
	expectDecimal(t, pre.InterestBeforeWindow, "-73.57", "InterestBeforeWindow (meses 2..60 del cuadro intacto)")

	// reduce-term: misma cuota, cierra antes, pierde también la cola.
	pre = mustPrepay(t, loan, rep, ReduceTerm)
	expectDecimal(t, pre.InterestSaved, "-1.46", "InterestSaved (subsidio renunciado, con signo)")

	for _, p := range []Prepayment{mustPrepay(t, loan, rep, ReducePayment), mustPrepay(t, loan, rep, ReduceTerm)} {
		if p.InterestSaved.Sign() > 0 {
			t.Errorf("%s: ahorro %s debe ser ≤ 0 con TIN negativo", string(p.Mode), p.InterestSaved.String())
		}
		// Identidad exacta del subsidio no devengado, con los campos públicos:
		// las filas hasta el evento son las del cuadro intacto, así que el
		// prefijo se lee del propio cuadro resultado.
		prefix := decimal.Zero
		for _, row := range p.Schedule.Rows[:rep.AfterMonth] {
			prefix = prefix.Add(row.Interest)
		}
		baseTail := p.InterestBefore.Sub(prefix) // vida intacta tras el evento
		afterTail := p.InterestAfter.Sub(prefix) // vida del cuadro nuevo
		if baseTail.Sign() > 0 || afterTail.Sign() > 0 || p.InterestBeforeWindow.Sign() > 0 {
			t.Errorf("%s: subsidio con signo equivocado: vida intacta %s, vida nueva %s, ventana %s",
				string(p.Mode), baseTail.String(), afterTail.String(), p.InterestBeforeWindow.String())
		}
		beyondWindow := baseTail.Sub(p.InterestBeforeWindow)
		if beyondWindow.Sign() > 0 {
			t.Errorf("%s: la cola intacta más allá de la ventana gana subsidio %s", string(p.Mode), beyondWindow.String())
		}
		want := p.InterestBeforeWindow.Sub(afterTail).Add(beyondWindow)
		if !p.InterestSaved.Equal(want) {
			t.Errorf("%s: ahorro %s != subsidio no devengado %s (ventana %s, vida nueva %s, cola %s)",
				string(p.Mode), p.InterestSaved.String(), want.String(),
				p.InterestBeforeWindow.String(), afterTail.String(), beyondWindow.String())
		}
		if p.InterestSaved.LessThan(baseTail) {
			t.Errorf("%s: la pérdida %s excede el subsidio íntegro de la vida intacta %s",
				string(p.Mode), p.InterestSaved.String(), baseTail.String())
		}
	}
}

// TestPrepayErrors: cada error centinela de la validación de entradas.
func TestPrepayErrors(t *testing.T) {
	loan := Loan{
		Principal:  mustDecimal(t, "150000.00"),
		AnnualRate: mustDecimal(t, "3.10"),
		Months:     300,
	}

	// Modo desconocido.
	_, err := Prepay(loan, EarlyRepayment{Amount: mustDecimal(t, "100"), AfterMonth: 60}, Mode("reduce-nothing"))
	if !errors.Is(err, ErrUnknownPrepayMode) {
		t.Errorf("modo desconocido: err = %v, se esperaba ErrUnknownPrepayMode", err)
	}

	// Importe negativo.
	_, err = Prepay(loan, EarlyRepayment{Amount: mustDecimal(t, "-0.01"), AfterMonth: 60}, ReducePayment)
	if !errors.Is(err, ErrNegativePrepayAmount) {
		t.Errorf("importe negativo: err = %v, se esperaba ErrNegativePrepayAmount", err)
	}

	// Comisión negativa.
	_, err = Prepay(loan, EarlyRepayment{Amount: mustDecimal(t, "100"), FeePct: mustDecimal(t, "-0.5"), AfterMonth: 60}, ReducePayment)
	if !errors.Is(err, ErrNegativeFeePct) {
		t.Errorf("comisión negativa: err = %v, se esperaba ErrNegativeFeePct", err)
	}

	// Mes fuera del cuadro: 0, negativo y más allá del cierre.
	for _, month := range []int{0, -1, 301} {
		_, err = Prepay(loan, EarlyRepayment{Amount: mustDecimal(t, "100"), AfterMonth: month}, ReducePayment)
		if !errors.Is(err, ErrAfterMonthOutsideSchedule) {
			t.Errorf("mes %d: err = %v, se esperaba ErrAfterMonthOutsideSchedule", month, err)
		}
	}

	// Importe mayor que el capital pendiente en ese mes: tras la cuota 60 el
	// saldo es 128506.54 (caso 1), así que 128506.55 no cabe; el importe
	// exactamente igual al saldo sí es válido y cierra el préstamo.
	_, err = Prepay(loan, EarlyRepayment{Amount: mustDecimal(t, "128506.55"), AfterMonth: 60}, ReducePayment)
	if !errors.Is(err, ErrAmountExceedsBalance) {
		t.Errorf("importe mayor que el saldo: err = %v, se esperaba ErrAmountExceedsBalance", err)
	}
	_, err = Prepay(loan, EarlyRepayment{Amount: mustDecimal(t, "128506.55"), AfterMonth: 60}, ReduceTerm)
	if !errors.Is(err, ErrAmountExceedsBalance) {
		t.Errorf("importe mayor que el saldo (reduce-term): err = %v, se esperaba ErrAmountExceedsBalance", err)
	}
	pre, err := Prepay(loan, EarlyRepayment{Amount: mustDecimal(t, "128506.54"), AfterMonth: 60}, ReduceTerm)
	if err != nil {
		t.Fatalf("importe igual al saldo: %v", err)
	}
	if pre.TermAfter != 60 || len(pre.Schedule.Rows) != 60 {
		t.Errorf("importe igual al saldo: TermAfter = %d, filas = %d; se esperaban 60 y 60 (cierre en el evento)",
			pre.TermAfter, len(pre.Schedule.Rows))
	}
	// El evento mismo cierra el préstamo: el cuadro termina en la fila del
	// evento (que muestra el saldo antes del evento) y BalanceAfter es 0.00
	// exacto.
	if !pre.BalanceAfter.IsZero() {
		t.Errorf("importe igual al saldo: BalanceAfter = %s, se esperaba 0.00 exacto", pre.BalanceAfter.String())
	}

	// El préstamo subyacente inválido propaga el error del núcleo.
	_, err = Prepay(Loan{Principal: mustDecimal(t, "150000"), AnnualRate: mustDecimal(t, "3.10"), Months: 0},
		EarlyRepayment{Amount: mustDecimal(t, "100"), AfterMonth: 1}, ReducePayment)
	if err == nil {
		t.Error("plazo 0: se esperaba un error del núcleo")
	}
}
