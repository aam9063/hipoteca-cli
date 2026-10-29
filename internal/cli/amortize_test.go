package cli

import (
	"encoding/json"
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

// El importe que agota el saldo cierra por el evento: closes_at_event en el
// JSON y ninguna cuota fantasma después (M1 de FIN-03).
func TestE2EAmortizeCierrePorElEvento(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	stdout, _, err := runRoot(t, deps, "amortize",
		"--principal", "150000", "--rate", "3.10", "--months", "300",
		"--amount", "128506.54", "--date", "60", "--mode", "reduce-term")
	if err != nil {
		t.Fatalf("amortize debe tener éxito, got %v", err)
	}
	var res app.AmortizeResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("el stdout no es el JSON del contrato: %v\n%s", err, stdout)
	}
	if res.ReduceTerm == nil || res.ReduceTerm.ClosesAtEvent == nil {
		t.Fatalf("la salida debe llevar closes_at_event: %+v", res)
	}
	if res.ReduceTerm.ClosesAtEvent.AfterMonth != 60 {
		t.Errorf("closes_at_event.after_month = %d, se esperaba 60", res.ReduceTerm.ClosesAtEvent.AfterMonth)
	}
	// La cuota mantenida es la del propio mes del evento, que sí se cobró: el
	// cuadro termina ahí (term_months_after 60), sin filas fantasma después.
	if got := res.ReduceTerm.PaymentAfter.Decimal().StringFixed(2); got != "719.14" {
		t.Errorf("payment_after = %s, se esperaba 719.14 (la cuota del mes del evento)", got)
	}
	if res.ReduceTerm.TermAfter != 60 {
		t.Errorf("term_months_after = %d, se esperaba 60", res.ReduceTerm.TermAfter)
	}
}

// --date es el mes del evento (índice de cuota), no una fecha de calendario
// (decisión 3): la ayuda de la bandera lo dice.
func TestE2EAmortizeLaAyudaDeDateExplicaQueEsUnIndiceDeCuota(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	stdout, _, err := runRoot(t, deps, "amortize", "--help")
	if err != nil {
		t.Fatalf("amortize --help debe tener éxito, got %v", err)
	}
	if !strings.Contains(stdout, "no es una fecha de calendario") {
		t.Errorf("la ayuda de --date debe explicar que es el índice de la cuota, no una fecha:\n%s", stdout)
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
