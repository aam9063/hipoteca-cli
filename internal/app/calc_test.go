package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// fechaEjecucion es la fecha fija que los tests pasan a Calc: la fecha de
// elaboración que la atribución debe llevar.
var fechaEjecucion = time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)

// calcInput es la parte de los casos de referencia de testdata/finance que
// este test necesita: las entradas y las cifras esperadas al céntimo.
type calcInput struct {
	ID     string `json:"id"`
	Inputs struct {
		Principal     string `json:"principal"`
		AnnualRatePct string `json:"annual_rate_pct"`
		TermMonths    int    `json:"term_months"`
	} `json:"inputs"`
	Expected struct {
		Payment       string `json:"payment"`
		TotalInterest string `json:"total_interest"`
		TotalPaid     string `json:"total_paid"`
	} `json:"expected"`
}

func readFinanceCase(t *testing.T, name string) calcInput {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "finance", name))
	if err != nil {
		t.Fatalf("leer el caso de referencia %s: %v", name, err)
	}
	var c calcInput
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("decodificar %s: %v", name, err)
	}
	return c
}

// calc reproduce los casos de referencia de finance al céntimo (CMD-01.3):
// los mismos ficheros que validan internal/finance, pasando por la capa de
// salida. La comparación es StringFixed(2): valor exacto y dos decimales.
func TestCalcReproduceLosCasosDeReferencia(t *testing.T) {
	cases := []string{
		"case-01-fijo-150000-300m.json",
		"case-02-cero-120000-240m.json",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			c := readFinanceCase(t, name)
			principal, err := model.NewDecimal(c.Inputs.Principal)
			if err != nil {
				t.Fatalf("capital del caso: %v", err)
			}
			rate, err := model.NewDecimal(c.Inputs.AnnualRatePct)
			if err != nil {
				t.Fatalf("tipo del caso: %v", err)
			}
			res, err := Calc(principal, rate, c.Inputs.TermMonths, fechaEjecucion)
			if err != nil {
				t.Fatalf("Calc: %v", err)
			}
			if res.SchemaVersion != model.SchemaVersion {
				t.Errorf("schema_version = %d, se esperaba %d", res.SchemaVersion, model.SchemaVersion)
			}
			if got := res.Payment.Decimal().StringFixed(2); got != c.Expected.Payment {
				t.Errorf("payment = %s, se esperaba %s", got, c.Expected.Payment)
			}
			if got := res.TotalInterest.Decimal().StringFixed(2); got != c.Expected.TotalInterest {
				t.Errorf("total_interest = %s, se esperaba %s", got, c.Expected.TotalInterest)
			}
			if got := res.TotalPaid.Decimal().StringFixed(2); got != c.Expected.TotalPaid {
				t.Errorf("total_paid = %s, se esperaba %s", got, c.Expected.TotalPaid)
			}
			// Las entradas se devuelven tal cual: la salida incluye sus
			// entradas (decisión 2 de CMD-01).
			if res.Inputs.Principal.String() != c.Inputs.Principal {
				t.Errorf("inputs.principal = %s, se esperaba %s", res.Inputs.Principal.String(), c.Inputs.Principal)
			}
			if res.Inputs.AnnualRatePct.String() != c.Inputs.AnnualRatePct {
				t.Errorf("inputs.annual_rate_pct = %s, se esperaba %s", res.Inputs.AnnualRatePct.String(), c.Inputs.AnnualRatePct)
			}
			if res.Inputs.TermMonths != c.Inputs.TermMonths {
				t.Errorf("inputs.term_months = %d, se esperaba %d", res.Inputs.TermMonths, c.Inputs.TermMonths)
			}
			// Atribución de datos procesados (P-05), fechada al día de la
			// ejecución: calc no consulta ningún dato del Banco, así que su
			// data_updated_at es la fecha de elaboración, no la de los datos.
			want, err := model.ProcessedAttribution("2026-02-14")
			if err != nil {
				t.Fatalf("construir la atribución esperada: %v", err)
			}
			if res.Attribution != want {
				t.Errorf("attribution = %+v, se esperaba la de datos procesados con data_updated_at %s: %+v", res.Attribution, "2026-02-14", want)
			}
		})
	}
}

// data_updated_at de calc es la fecha de elaboración del resultado (el día de
// la ejecución, en UTC), no la de ningún dato: calc no consulta al Banco de
// España. La hora de entrada no es UTC y cruza la medianoche al normalizar:
// debe ganar el día UTC.
func TestCalcDataUpdatedAtEsLaFechaDeEjecucion(t *testing.T) {
	principal, _ := model.NewDecimal("10000")
	rate, _ := model.NewDecimal("3.10")
	// 23:30 del 14 de febrero en UTC-2 ya es el 15 de febrero en UTC.
	now := time.Date(2026, 2, 14, 23, 30, 0, 0, time.FixedZone("UTC-2", -2*3600))
	res, err := Calc(principal, rate, 120, now)
	if err != nil {
		t.Fatalf("Calc: %v", err)
	}
	if res.Attribution.DataUpdatedAt != "2026-02-15" {
		t.Errorf("data_updated_at = %q, se esperaba 2026-02-15 (día de la ejecución en UTC)", res.Attribution.DataUpdatedAt)
	}
	if !res.Attribution.NotEndorsed {
		t.Error("not_endorsed debe ser true (P-05)")
	}
}

// Las entradas que finance rechaza son errores de uso (código 2): argumentos
// inválidos del usuario, no fallos internos.
func TestCalcErroresDeEntradaSonUsage(t *testing.T) {
	principal, _ := model.NewDecimal("150000")
	rate, _ := model.NewDecimal("3.10")
	negative, _ := model.NewDecimal("-1")

	cases := []struct {
		name      string
		principal model.Decimal
		months    int
	}{
		{"capital negativo", negative, 300},
		{"plazo cero", principal, 0},
		{"plazo negativo", principal, -5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Calc(tc.principal, rate, tc.months, fechaEjecucion)
			var e *errs.Error
			if !errors.As(err, &e) {
				t.Fatalf("el error debe llevar el sobre del contrato, got %v", err)
			}
			if e.Kind != errs.KindUsage {
				t.Errorf("kind = %q, se esperaba usage", e.Kind)
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
