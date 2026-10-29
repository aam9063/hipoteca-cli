package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// fechaAmortize es la fecha fija que los tests pasan a Amortize: la fecha de
// elaboración que la atribución debe llevar.
var fechaAmortize = time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)

// mustAppDecimal parsea s o corta el test.
func mustAppDecimal(t *testing.T, s string) model.Decimal {
	t.Helper()
	d, err := model.NewDecimal(s)
	if err != nil {
		t.Fatalf("decimal %q: %v", s, err)
	}
	return d
}

// prepayCase es la parte de los casos de referencia 9 y 10 de
// testdata/finance que este test necesita.
type prepayCase struct {
	ID     string `json:"id"`
	Inputs struct {
		Principal      string `json:"principal"`
		AnnualRatePct  string `json:"annual_rate_pct"`
		TermMonths     int    `json:"term_months"`
		EarlyRepayment struct {
			Mode              string `json:"mode"`
			AfterMonth        int    `json:"after_month"`
			Amount            string `json:"amount"`
			FeePct            string `json:"fee_pct"`
			OutstandingBefore string `json:"outstanding_before_event"`
			OutstandingAfter  string `json:"outstanding_after_event"`
			PaymentAfter      string `json:"payment_after_event"`
			TermAfter         int    `json:"term_months_after_event"`
			MonthsSaved       int    `json:"months_saved"`
		} `json:"early_repayment"`
	} `json:"inputs"`
	Expected struct {
		PaymentBefore   string `json:"payment_before_event"`
		InterestWithout string `json:"total_interest_without_early_repayment"`
		InterestSaved   string `json:"interest_saved"`
	} `json:"expected"`
}

func readPrepayCase(t *testing.T, name string) prepayCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "finance", name))
	if err != nil {
		t.Fatalf("leer el caso de referencia %s: %v", name, err)
	}
	var c prepayCase
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("decodificar %s: %v", name, err)
	}
	return c
}

// amortizeInputsDelCaso monta las entradas de Amortize desde un caso de
// referencia.
func amortizeInputsDelCaso(t *testing.T, c prepayCase, mode string) AmortizeInputs {
	t.Helper()
	return AmortizeInputs{
		Principal:     mustAppDecimal(t, c.Inputs.Principal),
		AnnualRatePct: mustAppDecimal(t, c.Inputs.AnnualRatePct),
		TermMonths:    c.Inputs.TermMonths,
		Amount:        mustAppDecimal(t, c.Inputs.EarlyRepayment.Amount),
		AfterMonth:    c.Inputs.EarlyRepayment.AfterMonth,
		Mode:          mode,
		FeePct:        mustAppDecimal(t, "0"),
	}
}

// bloqueDeElige el bloque de resultado del modo pedido.
func bloqueDe(t *testing.T, res AmortizeResult, mode string) *AmortizeOutcome {
	t.Helper()
	switch mode {
	case "reduce-payment":
		if res.ReducePayment == nil {
			t.Fatal("la salida no trae el bloque reduce_payment")
		}
		return res.ReducePayment
	case "reduce-term":
		if res.ReduceTerm == nil {
			t.Fatal("la salida no trae el bloque reduce_term")
		}
		return res.ReduceTerm
	}
	t.Fatalf("modo de prueba no soportado: %q", mode)
	return nil
}

// amortize reproduce los casos de referencia 9 y 10 de testdata/finance al
// céntimo (CMD-02.1): cuota tras el evento, plazo, ahorro de intereses y
// saldos, pasando por la capa de salida.
func TestAmortizeReproduceLosCasosDeReferencia(t *testing.T) {
	cases := []string{
		"case-09-amortiza-cuota.json",
		"case-10-amortiza-plazo.json",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			c := readPrepayCase(t, name)
			mode := c.Inputs.EarlyRepayment.Mode
			res, err := Amortize(amortizeInputsDelCaso(t, c, mode), fechaAmortize)
			if err != nil {
				t.Fatalf("Amortize: %v", err)
			}
			out := bloqueDe(t, res, mode)
			if out.Mode != mode {
				t.Errorf("mode = %q, se esperaba %q", out.Mode, mode)
			}
			if got := out.PaymentBefore.Decimal().StringFixed(2); got != c.Expected.PaymentBefore {
				t.Errorf("payment_before = %s, se esperaba %s", got, c.Expected.PaymentBefore)
			}
			if got := out.PaymentAfter.Decimal().StringFixed(2); got != c.Inputs.EarlyRepayment.PaymentAfter {
				t.Errorf("payment_after = %s, se esperaba %s", got, c.Inputs.EarlyRepayment.PaymentAfter)
			}
			if out.TermBefore != c.Inputs.TermMonths {
				t.Errorf("term_months_before = %d, se esperaba %d", out.TermBefore, c.Inputs.TermMonths)
			}
			if out.TermAfter != c.Inputs.EarlyRepayment.TermAfter {
				t.Errorf("term_months_after = %d, se esperaba %d", out.TermAfter, c.Inputs.EarlyRepayment.TermAfter)
			}
			if got := out.InterestSaved.Decimal().StringFixed(2); got != c.Expected.InterestSaved {
				t.Errorf("interest_saved = %s, se esperaba %s", got, c.Expected.InterestSaved)
			}
			if out.MonthsSaved != c.Inputs.EarlyRepayment.MonthsSaved {
				t.Errorf("months_saved = %d, se esperaba %d", out.MonthsSaved, c.Inputs.EarlyRepayment.MonthsSaved)
			}
			if got := out.BalanceBefore.Decimal().StringFixed(2); got != c.Inputs.EarlyRepayment.OutstandingBefore {
				t.Errorf("balance_before_event = %s, se esperaba %s", got, c.Inputs.EarlyRepayment.OutstandingBefore)
			}
			if got := out.BalanceAfter.Decimal().StringFixed(2); got != c.Inputs.EarlyRepayment.OutstandingAfter {
				t.Errorf("balance_after_event = %s, se esperaba %s", got, c.Inputs.EarlyRepayment.OutstandingAfter)
			}
			// Comisión 0 % del caso: fee 0.00 y desembolso = importe.
			if got := out.Fee.Decimal().StringFixed(2); got != "0.00" {
				t.Errorf("fee = %s, se esperaba 0.00", got)
			}
			if got := out.Outlay.Decimal().StringFixed(2); got != "10000.00" {
				t.Errorf("outlay = %s, se esperaba 10000.00", got)
			}
			// Los casos 9 y 10 no agotan el saldo: nada de cierre por evento.
			if out.ClosesAtEvent != nil {
				t.Errorf("closes_at_event = %+v, se esperaba ausente", *out.ClosesAtEvent)
			}
			// Las entradas se devuelven tal cual y el sobre es del contrato.
			if res.SchemaVersion != model.SchemaVersion {
				t.Errorf("schema_version = %d, se esperaba %d", res.SchemaVersion, model.SchemaVersion)
			}
			if res.Inputs.Principal.String() != c.Inputs.Principal {
				t.Errorf("inputs.principal = %s, se esperaba %s", res.Inputs.Principal.String(), c.Inputs.Principal)
			}
			if res.Inputs.Amount.String() != c.Inputs.EarlyRepayment.Amount {
				t.Errorf("inputs.amount = %s, se esperaba %s", res.Inputs.Amount.String(), c.Inputs.EarlyRepayment.Amount)
			}
			if res.Inputs.AfterMonth != c.Inputs.EarlyRepayment.AfterMonth {
				t.Errorf("inputs.after_month = %d, se esperaba %d", res.Inputs.AfterMonth, c.Inputs.EarlyRepayment.AfterMonth)
			}
			if res.Inputs.Mode != mode {
				t.Errorf("inputs.mode = %q, se esperaba %q", res.Inputs.Mode, mode)
			}
			want, err := model.ProcessedAttribution("2026-10-28")
			if err != nil {
				t.Fatalf("construir la atribución esperada: %v", err)
			}
			if res.Attribution != want {
				t.Errorf("attribution = %+v, se esperaba %+v", res.Attribution, want)
			}
		})
	}
}

// palabrasProhibidas son las que no pueden aparecer NUNCA en la salida de
// amortize y scenario (P-08): el comando calcula y compara, no recomienda.
var palabrasProhibidas = []string{"conviene", "mejor", "recomend", "deberías"}

// sinLenguajeDeRecomendacion comprueba que el JSON marshalled no lleva ni una
// palabra de asesoramiento.
func sinLenguajeDeRecomendacion(t *testing.T, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal de la salida: %v", err)
	}
	lower := strings.ToLower(string(data))
	for _, palabra := range palabrasProhibidas {
		if strings.Contains(lower, palabra) {
			t.Errorf("la salida contiene la palabra prohibida %q:\n%s", palabra, data)
		}
	}
}

// compare devuelve las dos opciones con sus cifras y nada más (decisión 1):
// dos bloques paralelos y ningún texto (ni campo) que recomiende.
func TestAmortizeCompareDevuelveAmbasOpcionesSinRecomendar(t *testing.T) {
	c := readPrepayCase(t, "case-09-amortiza-cuota.json")
	res, err := Amortize(amortizeInputsDelCaso(t, c, "compare"), fechaAmortize)
	if err != nil {
		t.Fatalf("Amortize compare: %v", err)
	}
	if res.ReducePayment == nil {
		t.Fatal("compare debe traer el bloque reduce_payment")
	}
	if res.ReduceTerm == nil {
		t.Fatal("compare debe traer el bloque reduce_term")
	}
	if res.ReducePayment.Mode != "reduce-payment" || res.ReduceTerm.Mode != "reduce-term" {
		t.Errorf("modos de los bloques = %q / %q, se esperaba reduce-payment / reduce-term",
			res.ReducePayment.Mode, res.ReduceTerm.Mode)
	}
	// Cada bloque lleva las cifras de su caso de referencia.
	if got := res.ReducePayment.PaymentAfter.Decimal().StringFixed(2); got != "663.18" {
		t.Errorf("reduce_payment.payment_after = %s, se esperaba 663.18", got)
	}
	if got := res.ReduceTerm.InterestSaved.Decimal().StringFixed(2); got != "8004.38" {
		t.Errorf("reduce_term.interest_saved = %s, se esperaba 8004.38 (case-10)", got)
	}
	sinLenguajeDeRecomendacion(t, res)
}

// M1 (FIN-03): cuando el importe agota el saldo, la salida marca el cierre por
// el evento con el mes del evento y no presenta ninguna cuota posterior que no
// existe: payment_after 0.00, term_months_after = mes del evento y
// balance_after_event 0.00.
func TestAmortizeImporteQueAgotaElSaldoCierraPorElEvento(t *testing.T) {
	c := readPrepayCase(t, "case-09-amortiza-cuota.json")
	for _, mode := range []string{"reduce-payment", "reduce-term"} {
		t.Run(mode, func(t *testing.T) {
			in := amortizeInputsDelCaso(t, c, mode)
			// El saldo en el mes 60 es 128506.54: amortizarlo exacto cierra.
			in.Amount = mustAppDecimal(t, c.Inputs.EarlyRepayment.OutstandingBefore)
			res, err := Amortize(in, fechaAmortize)
			if err != nil {
				t.Fatalf("Amortize: %v", err)
			}
			out := bloqueDe(t, res, mode)
			if out.ClosesAtEvent == nil {
				t.Fatal("la salida debe llevar el marcador closes_at_event")
			}
			if out.ClosesAtEvent.AfterMonth != c.Inputs.EarlyRepayment.AfterMonth {
				t.Errorf("closes_at_event.after_month = %d, se esperaba %d",
					out.ClosesAtEvent.AfterMonth, c.Inputs.EarlyRepayment.AfterMonth)
			}
			// Ninguna cuota existe tras el evento: el plazo termina en el mes
			// del evento. En reduce-payment no hay cuota nueva que recalcular
			// (0.00); en reduce-term la cuota mantenida es la del propio mes del
			// evento, que sí se cobró — el cuadro termina ahí, sin filas
			// fantasma después.
			wantPaymentAfter := "0.00"
			if mode == "reduce-term" {
				wantPaymentAfter = c.Expected.PaymentBefore
			}
			if got := out.PaymentAfter.Decimal().StringFixed(2); got != wantPaymentAfter {
				t.Errorf("payment_after = %s, se esperaba %s", got, wantPaymentAfter)
			}
			if out.TermAfter != c.Inputs.EarlyRepayment.AfterMonth {
				t.Errorf("term_months_after = %d, se esperaba %d (el mes del evento)",
					out.TermAfter, c.Inputs.EarlyRepayment.AfterMonth)
			}
			if got := out.BalanceAfter.Decimal().StringFixed(2); got != "0.00" {
				t.Errorf("balance_after_event = %s, se esperaba 0.00", got)
			}
			// El marcador debe llegar al JSON con su nombre del contrato.
			data, err := json.Marshal(res)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !strings.Contains(string(data), `"closes_at_event"`) {
				t.Errorf("el JSON debe llevar closes_at_event:\n%s", data)
			}
		})
	}
}

// Decisión 1 de FIN-03, ahora alcanzable desde el comando: la comisión se
// paga aparte y NO reduce el capital. El mismo importe con comisión 0 % y con
// 1 % produce exactamente la misma cuota, el mismo plazo, el mismo ahorro de
// intereses y el mismo saldo tras el evento; sólo cambian fee, outlay y el
// desembolso total del usuario.
func TestAmortizeLaComisionNoTocaElSaldo(t *testing.T) {
	c := readPrepayCase(t, "case-09-amortiza-cuota.json")
	sinComision := amortizeInputsDelCaso(t, c, "reduce-payment")
	conComision := amortizeInputsDelCaso(t, c, "reduce-payment")
	conComision.FeePct = mustAppDecimal(t, "1")

	sin, err := Amortize(sinComision, fechaAmortize)
	if err != nil {
		t.Fatalf("Amortize sin comisión: %v", err)
	}
	con, err := Amortize(conComision, fechaAmortize)
	if err != nil {
		t.Fatalf("Amortize con comisión: %v", err)
	}
	a, b := bloqueDe(t, sin, "reduce-payment"), bloqueDe(t, con, "reduce-payment")
	iguales := []struct {
		nombre string
		valor  [2]string
	}{
		{"payment_after", [2]string{a.PaymentAfter.Decimal().StringFixed(2), b.PaymentAfter.Decimal().StringFixed(2)}},
		{"term_months_after", [2]string{fmt.Sprintf("%d", a.TermAfter), fmt.Sprintf("%d", b.TermAfter)}},
		{"interest_saved", [2]string{a.InterestSaved.Decimal().StringFixed(2), b.InterestSaved.Decimal().StringFixed(2)}},
		{"balance_after_event", [2]string{a.BalanceAfter.Decimal().StringFixed(2), b.BalanceAfter.Decimal().StringFixed(2)}},
	}
	for _, campo := range iguales {
		if campo.valor[0] != campo.valor[1] {
			t.Errorf("%s cambió con la comisión: %s vs %s (debe ser idéntico)",
				campo.nombre, campo.valor[0], campo.valor[1])
		}
	}
	// 10000.00 con 1 %: comisión 100.00, desembolso 10100.00.
	if got := b.Fee.Decimal().StringFixed(2); got != "100.00" {
		t.Errorf("fee = %s, se esperaba 100.00", got)
	}
	if got := b.Outlay.Decimal().StringFixed(2); got != "10100.00" {
		t.Errorf("outlay = %s, se esperaba 10100.00", got)
	}
	if got := a.Fee.Decimal().StringFixed(2); got != "0.00" {
		t.Errorf("fee sin comisión = %s, se esperaba 0.00", got)
	}
	if got := a.Outlay.Decimal().StringFixed(2); got != "10000.00" {
		t.Errorf("outlay sin comisión = %s, se esperaba 10000.00", got)
	}
	// El eco de la entrada lleva la comisión tal cual.
	if con.Inputs.FeePct.String() != "1" {
		t.Errorf("inputs.fee_pct = %s, se esperaba 1", con.Inputs.FeePct.String())
	}
}

// Las entradas que finance rechaza son errores de uso (código 2).
func TestAmortizeErroresDeEntradaSonUsage(t *testing.T) {
	c := readPrepayCase(t, "case-09-amortiza-cuota.json")

	excede := amortizeInputsDelCaso(t, c, "reduce-payment")
	excede.Amount = mustAppDecimal(t, "999999.99") // supera el saldo del mes 60

	fuera := amortizeInputsDelCaso(t, c, "reduce-payment")
	fuera.AfterMonth = 0

	tarde := amortizeInputsDelCaso(t, c, "reduce-payment")
	tarde.AfterMonth = c.Inputs.TermMonths + 1

	negativo := amortizeInputsDelCaso(t, c, "reduce-payment")
	negativo.Amount = mustAppDecimal(t, "-1")

	plazoCero := amortizeInputsDelCaso(t, c, "reduce-payment")
	plazoCero.TermMonths = 0

	cases := []struct {
		name string
		in   AmortizeInputs
	}{
		{"modo desconocido", amortizeInputsDelCaso(t, c, "reduce-interest")},
		{"importe que supera el saldo", excede},
		{"mes del evento 0", fuera},
		{"mes del evento fuera del cuadro", tarde},
		{"importe negativo", negativo},
		{"plazo cero", plazoCero},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Amortize(tc.in, fechaAmortize)
			var e *errs.Error
			if !errors.As(err, &e) {
				t.Fatalf("el error debe llevar el sobre del contrato, got %v", err)
			}
			if e.Kind != errs.KindUsage {
				t.Errorf("kind = %q, se esperaba usage (mensaje: %s)", e.Kind, e.Message)
			}
			if errs.ExitCode(e.Kind) != 2 {
				t.Errorf("código de salida = %d, se esperaba 2", errs.ExitCode(e.Kind))
			}
			if strings.TrimSpace(e.Message) == "" {
				t.Error("el mensaje debe explicar qué entrada es inválida")
			}
		})
	}
}
