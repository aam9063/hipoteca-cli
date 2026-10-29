package finance

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
)

// Tests de regresión duraderos de FIN-03, añadidos a partir de la revisión
// independiente (verificador externo, PASS con esta observación): cubren
// propiedades que los casos de referencia y la rejilla no fijaban por sí
// solas — la comisión no toca la economía, el cierre a céntimo del
// reduce-term, el valor de InterestBeforeWindow contra un cuadro calculado
// aquí mismo, y la ausencia de resultados parciales junto a los errores
// centinela.

// TestPrepayFeeDoesNotTouchEconomics: la comisión se paga aparte (decisión 1),
// así que el mismo importe con comisión 0 %, 0.5 % o 3 % produce EXACTAMENTE
// la misma economía: misma cuota posterior, mismo plazo, mismo ahorro y fila a
// fila el mismo cuadro. Sólo cambian Fee y Outlay (= importe + comisión).
func TestPrepayFeeDoesNotTouchEconomics(t *testing.T) {
	loan := Loan{
		Principal:  mustDecimal(t, "150000.00"),
		AnnualRate: mustDecimal(t, "3.10"),
		Months:     300,
	}

	var base Prepayment
	for i, feePct := range []string{"0", "0.5", "3"} {
		pre := mustPrepay(t, loan, EarlyRepayment{
			Amount:     mustDecimal(t, "10000.00"),
			FeePct:     mustDecimal(t, feePct),
			AfterMonth: 60,
		}, ReducePayment)

		expectDecimal(t, pre.Fee, map[string]string{"0": "0.00", "0.5": "50.00", "3": "300.00"}[feePct],
			"Fee ("+feePct+" % de 10000.00)")
		expectDecimal(t, pre.Outlay, mustDecimal(t, "10000.00").Add(pre.Fee).String(), "Outlay == importe + comisión")

		// El capital baja exactamente por el importe, sea cual sea la
		// comisión (decisión 1).
		expectDecimal(t, pre.BalanceBefore, "128506.54", "BalanceBefore")
		expectDecimal(t, pre.BalanceAfter, "118506.54", "BalanceAfter")

		if i == 0 {
			base = pre
			continue
		}
		// Misma economía que la variante sin comisión: cuota, plazo, ahorro
		// y cada fila del cuadro.
		if !pre.PaymentAfter.Equal(base.PaymentAfter) {
			t.Errorf("fee %s %%: cuota %s != %s sin comisión", feePct, pre.PaymentAfter.String(), base.PaymentAfter.String())
		}
		if pre.TermAfter != base.TermAfter || pre.TermBefore != base.TermBefore {
			t.Errorf("fee %s %%: plazo %d/%d != %d/%d sin comisión",
				feePct, pre.TermBefore, pre.TermAfter, base.TermBefore, base.TermAfter)
		}
		if !pre.InterestSaved.Equal(base.InterestSaved) {
			t.Errorf("fee %s %%: ahorro %s != %s sin comisión", feePct, pre.InterestSaved.String(), base.InterestSaved.String())
		}
		if len(pre.Schedule.Rows) != len(base.Schedule.Rows) {
			t.Fatalf("fee %s %%: el cuadro tiene %d filas, sin comisión %d", feePct, len(pre.Schedule.Rows), len(base.Schedule.Rows))
		}
		for k, row := range pre.Schedule.Rows {
			want := base.Schedule.Rows[k]
			if row.Month != want.Month || !row.Payment.Equal(want.Payment) || !row.Interest.Equal(want.Interest) ||
				!row.Principal.Equal(want.Principal) || !row.Balance.Equal(want.Balance) {
				t.Errorf("fee %s %%: fila %d = %+v, sin comisión %+v", feePct, row.Month, row, want)
			}
		}

		// El cuadro más el importe anticipado devuelve el capital: la
		// comisión no entra en el cuadro.
		sumPrincipal := decimal.Zero
		for _, row := range pre.Schedule.Rows {
			sumPrincipal = sumPrincipal.Add(row.Principal)
		}
		if !sumPrincipal.Add(pre.Amount).Equal(loan.Principal) {
			t.Errorf("fee %s %%: amortizaciones %s + importe %s != capital %s",
				feePct, sumPrincipal.String(), pre.Amount.String(), loan.Principal.String())
		}
	}
}

// TestPrepayReduceTermExactCentResiduals: el cierre a céntimo del reduce-term
// con tipo 0 %, donde toda la aritmética es visible a mano. Préstamo
// 120000.00 a 0 % a 240 meses: cuota exacta 500.00, saldo tras la cuota 1
// = 119500.00.
//
//	amortiza 14000.00 → saldo 105500.00 = 211 · 500.00 justo:
//	  plazo 212 en total (1 + 211) y última cuota 500.00 sin residuo.
//	amortiza 14000.01 → saldo 105499.99 = 210 · 500.00 + 499.99:
//	  mismo plazo 212 y última cuota 499.99: la última absorbe el residuo.
//
// El saldo de la penúltima fila es 500.00 (> 0) en ambos casos, lo que prueba
// que el plazo es el mínimo: con un mes menos quedaría capital sin pagar.
func TestPrepayReduceTermExactCentResiduals(t *testing.T) {
	loan := Loan{Principal: mustDecimal(t, "120000.00"), AnnualRate: mustDecimal(t, "0"), Months: 240}

	cases := []struct {
		amount      string
		wantLastPay string
	}{
		{amount: "14000.00", wantLastPay: "500.00"},
		{amount: "14000.01", wantLastPay: "499.99"},
	}
	for _, c := range cases {
		t.Run(c.amount, func(t *testing.T) {
			pre := mustPrepay(t, loan, EarlyRepayment{
				Amount:     mustDecimal(t, c.amount),
				AfterMonth: 1,
			}, ReduceTerm)

			if pre.TermAfter != 212 || pre.MonthsSaved != 28 {
				t.Errorf("TermAfter = %d, MonthsSaved = %d; se esperaban 212 y 28", pre.TermAfter, pre.MonthsSaved)
			}
			if !pre.InterestSaved.IsZero() || !pre.InterestAfter.IsZero() {
				t.Errorf("a 0 %% no hay interés: ahorro %s, interés después %s",
					pre.InterestSaved.String(), pre.InterestAfter.String())
			}
			rows := pre.Schedule.Rows
			if len(rows) != 212 {
				t.Fatalf("filas = %d, se esperaban 212", len(rows))
			}
			last := rows[len(rows)-1]
			expectDecimal(t, last.Payment, c.wantLastPay, "última cuota")
			if !last.Interest.IsZero() {
				t.Errorf("última cuota: interés %s, se esperaba 0.00 a 0 %%", last.Interest.String())
			}
			if !last.Balance.IsZero() {
				t.Errorf("capital final = %s, se esperaba 0.00 exacto", last.Balance.String())
			}
			// Plazo mínimo: la penúltima fila deja capital pendiente (> 0, y
			// justo el que la última cuota consume), así que ningún mes anterior
			// puede ser el último: con un mes menos quedaría capital sin pagar.
			penultimate := rows[len(rows)-2]
			if !penultimate.Balance.Equal(mustDecimal(t, c.wantLastPay)) {
				t.Errorf("saldo penúltimo = %s, se esperaba %s (el que la última cuota consume)",
					penultimate.Balance.String(), c.wantLastPay)
			}
			if penultimate.Balance.Sign() <= 0 {
				t.Errorf("saldo penúltimo = %s: el plazo no es mínimo", penultimate.Balance.String())
			}
		})
	}
}

// TestPrepayInterestBeforeWindowAgainstFreshBaseline: InterestBeforeWindow
// (interés del cuadro intacto en la ventana que recorre el cuadro nuevo) se
// comprueba contra un Amortization fresco calculado aquí, no contra campos
// derivados del propio Prepay. Es la comprobación sustantiva que sustituye a
// la identidad algebraica retirada: IBW − vida nueva + cola fuera de ventana
// se reduce a baseTail − vida nueva (la ventana se cancela) y nunca habría
// detectado una InterestBeforeWindow mal calculada.
//
// Con TIN −0.30 % el interés es a favor del prestatario, así que de paso se
// suejeta el signo del ahorro en ambos modos.
func TestPrepayInterestBeforeWindowAgainstFreshBaseline(t *testing.T) {
	loan := Loan{
		Principal:  mustDecimal(t, "150000.00"),
		AnnualRate: mustDecimal(t, "-0.30"),
		Months:     300,
	}
	rep := EarlyRepayment{Amount: mustDecimal(t, "10000.00"), AfterMonth: 60}

	fresh, err := Amortization(loan)
	if err != nil {
		t.Fatalf("Amortization del préstamo base: %v", err)
	}

	for _, mode := range []Mode{ReducePayment, ReduceTerm} {
		pre := mustPrepay(t, loan, rep, mode)
		if pre.InterestSaved.Sign() > 0 {
			t.Errorf("%s: con TIN negativo el ahorro %s debe ser ≤ 0", string(mode), pre.InterestSaved.String())
		}
		if pre.TermAfter > len(fresh.Rows) {
			t.Fatalf("%s: TermAfter %d excede el cuadro intacto (%d filas)", string(mode), pre.TermAfter, len(fresh.Rows))
		}
		var want decimal.Decimal
		for _, row := range fresh.Rows[rep.AfterMonth:pre.TermAfter] {
			want = want.Add(row.Interest)
		}
		if !pre.InterestBeforeWindow.Equal(want) {
			t.Errorf("%s: InterestBeforeWindow = %s, el cuadro fresco da %s (meses %d..%d)",
				string(mode), pre.InterestBeforeWindow.String(), want.String(), rep.AfterMonth+1, pre.TermAfter)
		}
	}
}

// TestPrepayErrorsReturnZeroValue: junto a cada error centinela no hay
// resultado parcial: el Prepayment devuelto es el valor cero (sin filas, sin
// cifras), nunca un resultado a medias que un llamador despistado podría
// presentar.
func TestPrepayErrorsReturnZeroValue(t *testing.T) {
	loan := Loan{
		Principal:  mustDecimal(t, "150000.00"),
		AnnualRate: mustDecimal(t, "3.10"),
		Months:     300,
	}
	fee := mustDecimal(t, "0.5")

	cases := []struct {
		name string
		mode Mode
		rep  EarlyRepayment
		want error
	}{
		{"modo desconocido", Mode("reduce-nothing"), EarlyRepayment{Amount: mustDecimal(t, "100"), AfterMonth: 60}, ErrUnknownPrepayMode},
		{"importe negativo", ReducePayment, EarlyRepayment{Amount: mustDecimal(t, "-0.01"), AfterMonth: 60}, ErrNegativePrepayAmount},
		{"comisión negativa", ReducePayment, EarlyRepayment{Amount: mustDecimal(t, "100"), FeePct: mustDecimal(t, "-0.5"), AfterMonth: 60}, ErrNegativeFeePct},
		{"mes 0", ReducePayment, EarlyRepayment{Amount: mustDecimal(t, "100"), AfterMonth: 0}, ErrAfterMonthOutsideSchedule},
		{"mes 301", ReducePayment, EarlyRepayment{Amount: mustDecimal(t, "100"), AfterMonth: 301}, ErrAfterMonthOutsideSchedule},
		{"importe mayor que el saldo", ReducePayment, EarlyRepayment{Amount: mustDecimal(t, "128506.55"), FeePct: fee, AfterMonth: 60}, ErrAmountExceedsBalance},
		{"importe mayor que el saldo (reduce-term)", ReduceTerm, EarlyRepayment{Amount: mustDecimal(t, "128506.55"), FeePct: fee, AfterMonth: 60}, ErrAmountExceedsBalance},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pre, err := Prepay(loan, c.rep, c.mode)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, se esperaba %v", err, c.want)
			}
			if pre.Mode != "" || len(pre.Schedule.Rows) != 0 || pre.TermBefore != 0 || pre.TermAfter != 0 ||
				pre.Amount.Sign() != 0 || pre.Fee.Sign() != 0 || pre.Outlay.Sign() != 0 ||
				pre.BalanceBefore.Sign() != 0 || pre.BalanceAfter.Sign() != 0 ||
				pre.InterestBefore.Sign() != 0 || pre.InterestAfter.Sign() != 0 ||
				pre.InterestSaved.Sign() != 0 || pre.InterestBeforeWindow.Sign() != 0 ||
				pre.Schedule.TotalInterest.Sign() != 0 || pre.Schedule.TotalPaid.Sign() != 0 {
				t.Errorf("junto al error %v queda un resultado parcial: %+v", err, pre)
			}
		})
	}
}
