package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// `hipoteca amortize --principal 150000 --rate 3.10 --months 300 --amount
// 10000 --date 60 --mode reduce-payment` reproduce el caso de referencia 9 al
// céntimo, sin préstamo guardado y sin red.
func TestE2EAmortizeReproduceElCasoDeReferencia9(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	stdout, stderr, err := runRoot(t, deps, "amortize",
		"--principal", "150000", "--rate", "3.10", "--months", "300",
		"--amount", "10000", "--date", "60", "--mode", "reduce-payment")
	if err != nil {
		t.Fatalf("amortize debe tener éxito, got %v", err)
	}
	assertEmpty(t, "stderr", stderr)

	var res app.AmortizeResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("el stdout no es el JSON del contrato: %v\n%s", err, stdout)
	}
	if res.ReducePayment == nil {
		t.Fatal("la salida debe llevar el bloque reduce_payment")
	}
	if got := res.ReducePayment.PaymentAfter.Decimal().StringFixed(2); got != "663.18" {
		t.Errorf("payment_after = %s, se esperaba 663.18", got)
	}
	if got := res.ReducePayment.InterestSaved.Decimal().StringFixed(2); got != "3431.18" {
		t.Errorf("interest_saved = %s, se esperaba 3431.18", got)
	}
	if res.ReducePayment.TermAfter != 300 {
		t.Errorf("term_months_after = %d, se esperaba 300", res.ReducePayment.TermAfter)
	}
	if res.Inputs.Principal.String() != "150000" || res.Inputs.Amount.String() != "10000" || res.Inputs.AfterMonth != 60 {
		t.Errorf("inputs = %+v, se esperaba 150000 / 10000 / 60", res.Inputs)
	}
	if !res.Attribution.NotEndorsed {
		t.Error("not_endorsed debe ser true (P-05)")
	}
	assertSingleJSON(t, stdout)
}

// `--mode compare` devuelve las dos opciones y ningún texto que recomiende
// (P-08): el JSON marshalled no contiene «conviene», «mejor», «recomend» ni
// «deberías». El test falla si cualquier palabra de asesoramiento aparece.
func TestE2EAmortizeCompareDevuelveAmbosBloquesSinRecomendar(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	stdout, stderr, err := runRoot(t, deps, "amortize",
		"--principal", "150000", "--rate", "3.10", "--months", "300",
		"--amount", "10000", "--date", "60", "--mode", "compare")
	if err != nil {
		t.Fatalf("amortize compare debe tener éxito, got %v", err)
	}
	assertEmpty(t, "stderr", stderr)

	var res app.AmortizeResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("el stdout no es el JSON del contrato: %v\n%s", err, stdout)
	}
	if res.ReducePayment == nil || res.ReduceTerm == nil {
		t.Fatalf("compare debe traer los dos bloques: %+v", res)
	}
	if got := res.ReducePayment.PaymentAfter.Decimal().StringFixed(2); got != "663.18" {
		t.Errorf("reduce_payment.payment_after = %s, se esperaba 663.18", got)
	}
	if res.ReduceTerm.TermAfter != 275 {
		t.Errorf("reduce_term.term_months_after = %d, se esperaba 275", res.ReduceTerm.TermAfter)
	}

	lower := strings.ToLower(stdout)
	for _, palabra := range []string{"conviene", "mejor", "recomend", "deberías"} {
		if strings.Contains(lower, palabra) {
			t.Errorf("la salida contiene la palabra prohibida %q:\n%s", palabra, stdout)
		}
	}
	assertSingleJSON(t, stdout)
}

// El importe que agota el saldo cierra por el evento en los DOS modos:
// closes_at_event con sus campos y ninguna cuota fantasma después (M1 de
// FIN-03). En reduce-payment no hay cuota que recalcular (payment_after
// 0.00); en reduce-term la cuota mantenida es la del propio mes del evento.
func TestE2EAmortizeCierrePorElEventoEnAmbosModos(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	for _, mode := range []string{"reduce-payment", "reduce-term"} {
		t.Run(mode, func(t *testing.T) {
			stdout, _, err := runRoot(t, deps, "amortize",
				"--principal", "150000", "--rate", "3.10", "--months", "300",
				"--amount", "128506.54", "--date", "60", "--mode", mode)
			if err != nil {
				t.Fatalf("amortize debe tener éxito, got %v", err)
			}
			var res app.AmortizeResult
			if err := json.Unmarshal([]byte(stdout), &res); err != nil {
				t.Fatalf("el stdout no es el JSON del contrato: %v\n%s", err, stdout)
			}
			out := bloquePorNombre(t, res, mode)
			if out.ClosesAtEvent == nil {
				t.Fatalf("la salida debe llevar closes_at_event: %+v", res)
			}
			if out.ClosesAtEvent.AfterMonth != 60 {
				t.Errorf("closes_at_event.after_month = %d, se esperaba 60", out.ClosesAtEvent.AfterMonth)
			}
			if got := out.ClosesAtEvent.Amount.Decimal().StringFixed(2); got != "128506.54" {
				t.Errorf("closes_at_event.amount = %s, se esperaba 128506.54", got)
			}
			if got := out.ClosesAtEvent.BalanceBefore.Decimal().StringFixed(2); got != "128506.54" {
				t.Errorf("closes_at_event.balance_before_event = %s, se esperaba 128506.54", got)
			}
			// Ninguna cuota existe tras el evento: el plazo termina en el mes 60.
			if out.TermAfter != 60 {
				t.Errorf("term_months_after = %d, se esperaba 60 (sin cuotas fantasma)", out.TermAfter)
			}
			if got := out.BalanceAfter.Decimal().StringFixed(2); got != "0.00" {
				t.Errorf("balance_after_event = %s, se esperaba 0.00", got)
			}
			wantPayment := "0.00"
			if mode == "reduce-term" {
				wantPayment = "719.14"
			}
			if got := out.PaymentAfter.Decimal().StringFixed(2); got != wantPayment {
				t.Errorf("payment_after = %s, se esperaba %s", got, wantPayment)
			}
			// Dinero siempre a dos decimales en el JSON (verificador, minor 1).
			wantJSON := `"payment_after":"` + wantPayment + `"`
			if !strings.Contains(stdout, wantJSON) || !strings.Contains(stdout, `"balance_after_event":"0.00"`) {
				t.Errorf("el JSON debe llevar el dinero a dos decimales:\n%s", stdout)
			}
		})
	}
}

// bloquePorNombre devuelve el bloque del modo pedido.
func bloquePorNombre(t *testing.T, res app.AmortizeResult, mode string) *app.AmortizeOutcome {
	t.Helper()
	if mode == "reduce-payment" {
		if res.ReducePayment == nil {
			t.Fatal("falta el bloque reduce_payment")
		}
		return res.ReducePayment
	}
	if res.ReduceTerm == nil {
		t.Fatal("falta el bloque reduce_term")
	}
	return res.ReduceTerm
}

// La comisión es independiente del cálculo en los DOS modos (decisión 1 de
// FIN-03): el mismo cálculo con --fee-pct 0, 1 y 2.005 da idénticas cuota,
// plazo, ahorro y saldo tras el evento; sólo cambian fee y outlay.
func TestE2EAmortizeLaComisionEsIndependienteDelCalculo(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	bloquesPorFee := map[string]app.AmortizeResult{}
	for _, fee := range []string{"0", "1", "2.005"} {
		stdout, _, err := runRoot(t, deps, "amortize",
			"--principal", "150000", "--rate", "3.10", "--months", "300",
			"--amount", "10000", "--date", "60", "--mode", "compare", "--fee-pct", fee)
		if err != nil {
			t.Fatalf("amortize --fee-pct %s debe tener éxito, got %v", fee, err)
		}
		var res app.AmortizeResult
		if err := json.Unmarshal([]byte(stdout), &res); err != nil {
			t.Fatalf("el stdout no es el JSON del contrato: %v\n%s", err, stdout)
		}
		bloquesPorFee[fee] = res
	}
	base := bloquesPorFee["0"]
	for _, fee := range []string{"1", "2.005"} {
		r := bloquesPorFee[fee]
		for _, nombre := range []string{"reduce-payment", "reduce-term"} {
			a, b := bloquePorNombre(t, base, nombre), bloquePorNombre(t, r, nombre)
			campos := []struct {
				nombre string
				valor  [2]string
			}{
				{"payment_after", [2]string{a.PaymentAfter.Decimal().StringFixed(2), b.PaymentAfter.Decimal().StringFixed(2)}},
				{"term_months_after", [2]string{fmt.Sprintf("%d", a.TermAfter), fmt.Sprintf("%d", b.TermAfter)}},
				{"interest_saved", [2]string{a.InterestSaved.Decimal().StringFixed(2), b.InterestSaved.Decimal().StringFixed(2)}},
				{"balance_after_event", [2]string{a.BalanceAfter.Decimal().StringFixed(2), b.BalanceAfter.Decimal().StringFixed(2)}},
			}
			for _, campo := range campos {
				if campo.valor[0] != campo.valor[1] {
					t.Errorf("--fee-pct %s: %s cambió en %s: %s vs %s (debe ser idéntico)",
						fee, campo.nombre, nombre, campo.valor[0], campo.valor[1])
				}
			}
		}
	}
	// fee/outlay sí cambian, siempre a dos decimales.
	fees := map[string][2]string{
		"0":     {"0.00", "10000.00"},
		"1":     {"100.00", "10100.00"},
		"2.005": {"200.50", "10200.50"},
	}
	for fee, want := range fees {
		r := bloquePorNombre(t, bloquesPorFee[fee], "reduce-payment")
		if got := r.Fee.Decimal().StringFixed(2); got != want[0] {
			t.Errorf("--fee-pct %s: fee = %s, se esperaba %s", fee, got, want[0])
		}
		if got := r.Outlay.Decimal().StringFixed(2); got != want[1] {
			t.Errorf("--fee-pct %s: outlay = %s, se esperaba %s", fee, got, want[1])
		}
	}
}

// El redondeo half-up de la comisión al céntimo: el punto medio sube y lo que
// no llega baja (0.015 → 0.02, 0.01495 → 0.01).
func TestE2EAmortizeComisionPuntoMedio(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	cases := []struct {
		importe string
		want    string
	}{
		{"3.00", "0.02"},
		{"2.99", "0.01"},
	}
	for _, tc := range cases {
		t.Run(tc.importe, func(t *testing.T) {
			stdout, _, err := runRoot(t, deps, "amortize",
				"--principal", "150000", "--rate", "3.10", "--months", "300",
				"--amount", tc.importe, "--date", "60", "--mode", "reduce-payment", "--fee-pct", "0.5")
			if err != nil {
				t.Fatalf("amortize debe tener éxito, got %v", err)
			}
			var res app.AmortizeResult
			if err := json.Unmarshal([]byte(stdout), &res); err != nil {
				t.Fatalf("el stdout no es el JSON del contrato: %v\n%s", err, stdout)
			}
			if got := res.ReducePayment.Fee.Decimal().StringFixed(2); got != tc.want {
				t.Errorf("fee = %s, se esperaba %s", got, tc.want)
			}
		})
	}
}

// --date es el mes del evento (índice de cuota), no una fecha de calendario
// (decisión 3): la ayuda lo dice y explica qué guarda payment_after cuando el
// préstamo cierra en el evento.
func TestE2EAmortizeLaAyudaDeDateExplicaQueEsUnIndiceDeCuota(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	stdout, _, err := runRoot(t, deps, "amortize", "--help")
	if err != nil {
		t.Fatalf("amortize --help debe tener éxito, got %v", err)
	}
	if !strings.Contains(stdout, "no es una fecha de calendario") {
		t.Errorf("la ayuda de --date debe explicar que es el índice de la cuota, no una fecha:\n%s", stdout)
	}
	// Verificador, minor 2: payment_after significa cosas distintas en M1
	// según el modo; la ayuda debe explicarlo.
	for _, frase := range []string{"payment_after", "reduce-payment", "reduce-term"} {
		if !strings.Contains(stdout, frase) {
			t.Errorf("la ayuda debe explicar qué guarda payment_after en cada modo cuando el préstamo cierra por el evento:\n%s", stdout)
		}
	}
}

// --fee-pct: la comisión se paga aparte y no reduce el capital (decisión 1 de
// FIN-03). 10000.00 con 1 % → fee 100.00, outlay 10100.00, y la misma cuota,
// plazo y ahorro que con 0 %.
func TestE2EAmortizeLaComisionNoTocaElSaldo(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	base := []string{"amortize", "--principal", "150000", "--rate", "3.10", "--months", "300",
		"--amount", "10000", "--date", "60", "--mode", "reduce-payment"}

	sinOut, _, err := runRoot(t, deps, base...)
	if err != nil {
		t.Fatalf("amortize sin comisión debe tener éxito, got %v", err)
	}
	conOut, stderr, err := runRoot(t, deps, append(append([]string{}, base...), "--fee-pct", "1")...)
	if err != nil {
		t.Fatalf("amortize --fee-pct 1 debe tener éxito, got %v", err)
	}
	assertEmpty(t, "stderr", stderr)

	var sin, con app.AmortizeResult
	if err := json.Unmarshal([]byte(sinOut), &sin); err != nil {
		t.Fatalf("el stdout no es el JSON del contrato: %v\n%s", err, sinOut)
	}
	if err := json.Unmarshal([]byte(conOut), &con); err != nil {
		t.Fatalf("el stdout no es el JSON del contrato: %v\n%s", err, conOut)
	}
	if con.ReducePayment == nil || sin.ReducePayment == nil {
		t.Fatal("la salida debe llevar el bloque reduce_payment")
	}
	// La comisión no toca el saldo: mismas cifras que con 0 %.
	if got := con.ReducePayment.PaymentAfter.Decimal().StringFixed(2); got != "663.18" {
		t.Errorf("payment_after = %s, se esperaba 663.18 (idéntico al caso sin comisión)", got)
	}
	if con.ReducePayment.TermAfter != sin.ReducePayment.TermAfter {
		t.Errorf("term_months_after cambió con la comisión: %d vs %d", con.ReducePayment.TermAfter, sin.ReducePayment.TermAfter)
	}
	if con.ReducePayment.InterestSaved.Decimal().StringFixed(2) != sin.ReducePayment.InterestSaved.Decimal().StringFixed(2) {
		t.Errorf("interest_saved cambió con la comisión: %s vs %s",
			con.ReducePayment.InterestSaved.Decimal().StringFixed(2), sin.ReducePayment.InterestSaved.Decimal().StringFixed(2))
	}
	if got := con.ReducePayment.BalanceAfter.Decimal().StringFixed(2); got != "118506.54" {
		t.Errorf("balance_after_event = %s, se esperaba 118506.54 (la comisión no reduce el capital)", got)
	}
	// 10000.00 con 1 %: fee 100.00, outlay 10100.00.
	if got := con.ReducePayment.Fee.Decimal().StringFixed(2); got != "100.00" {
		t.Errorf("fee = %s, se esperaba 100.00", got)
	}
	if got := con.ReducePayment.Outlay.Decimal().StringFixed(2); got != "10100.00" {
		t.Errorf("outlay = %s, se esperaba 10100.00", got)
	}
	if con.Inputs.FeePct.String() != "1" {
		t.Errorf("inputs.fee_pct = %s, se esperaba 1", con.Inputs.FeePct.String())
	}
	assertSingleJSON(t, conOut)
}

// Faltan banderas, el modo no existe o una entrada no es válida: usage/2 con
// stdout vacío.
func TestE2EAmortizeEntradasInvalidas(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	cabeza := []string{"amortize", "--principal", "150000", "--rate", "3.10", "--months", "300"}
	cases := [][]string{
		{"amortize"},
		{"amortize", "--principal", "150000", "--rate", "3.10"},
		{"amortize", "--principal", "150000", "--rate", "3.10", "--months", "300",
			"--amount", "10000", "--date", "60"}, // falta --mode
		append(cabeza, "--amount", "10000", "--date", "60", "--mode", "reduce-interest"),
		append(cabeza, "--amount", "10000", "--date", "0", "--mode", "reduce-payment"),
		append(cabeza, "--amount", "999999.99", "--date", "60", "--mode", "reduce-payment"),
		append(cabeza, "--amount", "10000", "--date", "60", "--mode", "reduce-payment", "--principal", "abc"),
		append(cabeza, "--amount", "10000", "--date", "60", "--mode", "reduce-payment", "--fee-pct", "abc"),
		append(cabeza, "--amount", "10000", "--date", "60", "--mode", "reduce-payment", "--fee-pct", "-1"),
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, _, err := runRoot(t, deps, args...)
			assertContractError(t, err, errs.KindUsage, 2)
			assertEmpty(t, "stdout", stdout)
		})
	}
}
