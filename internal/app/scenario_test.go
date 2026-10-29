package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// fechaScenario es la fecha fija que los tests pasan a Scenario.
var fechaScenario = time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)

// scenarioEntradas monta las entradas de Scenario con los valores dados.
func scenarioEntradas(t *testing.T, principal, spread string, floor *model.Decimal, rounding string, months int) ScenarioInputs {
	t.Helper()
	in := ScenarioInputs{
		Principal:  mustAppDecimal(t, principal),
		TermMonths: months,
		Spread:     mustAppDecimal(t, spread),
		Rounding:   rounding,
	}
	in.Floor = floor
	return in
}

func scenarioValores(t *testing.T, valores ...string) []model.Decimal {
	t.Helper()
	out := make([]model.Decimal, 0, len(valores))
	for _, v := range valores {
		out = append(out, mustAppDecimal(t, v))
	}
	return out
}

// scenario reproduce el caso de referencia 1 como caso conocido (CMD-02.4):
// el índice 3.10 sin diferencial da el TIN 3.10 y la cuota 719.14 sobre
// 150000 a 300 meses, con su coste total. Cada valor se devuelve tal cual y
// marcado assumed (decisión 4, P-06).
func TestScenarioReproduceElCasoConocido(t *testing.T) {
	in := scenarioEntradas(t, "150000", "0", nil, "none", 300)
	res, err := Scenario(in, scenarioValores(t, "3.10"), fechaScenario)
	if err != nil {
		t.Fatalf("Scenario: %v", err)
	}
	if res.SchemaVersion != model.SchemaVersion {
		t.Errorf("schema_version = %d, se esperaba %d", res.SchemaVersion, model.SchemaVersion)
	}
	if len(res.Values) != 1 {
		t.Fatalf("values = %d bloques, se esperaba 1", len(res.Values))
	}
	v := res.Values[0]
	if got := v.IndexValue.String(); got != "3.10" {
		t.Errorf("index_value = %q, se esperaba \"3.10\" (ecoado tal cual)", got)
	}
	if !v.Assumed {
		t.Error("assumed debe ser true: el valor no viene de la API (P-06)")
	}
	if got := v.AnnualRatePct.Decimal().StringFixed(2); got != "3.10" {
		t.Errorf("annual_rate_pct = %s, se esperaba 3.10", got)
	}
	if got := v.Payment.Decimal().StringFixed(2); got != "719.14" {
		t.Errorf("payment = %s, se esperaba 719.14", got)
	}
	if got := v.TotalInterest.Decimal().StringFixed(2); got != "65743.56" {
		t.Errorf("total_interest = %s, se esperaba 65743.56", got)
	}
	if got := v.TotalPaid.Decimal().StringFixed(2); got != "215743.56" {
		t.Errorf("total_paid = %s, se esperaba 215743.56", got)
	}
	want, err := model.ProcessedAttribution("2026-10-28")
	if err != nil {
		t.Fatalf("construir la atribución esperada: %v", err)
	}
	if res.Attribution != want {
		t.Errorf("attribution = %+v, se esperaba %+v", res.Attribution, want)
	}
	// Las entradas se devuelven tal cual.
	if res.Inputs.Principal.String() != "150000" || res.Inputs.TermMonths != 300 {
		t.Errorf("inputs = %+v, se esperaba principal 150000 y 300 meses", res.Inputs)
	}
}

// La cadena NewRate se aplica a cada valor: diferencial, suelo y redondeo de
// escritura en el orden normativo (convención 1.5). El eco del valor mantiene
// la escala original: "1.50" se serializa "1.50", no "1.5".
func TestScenarioAplicaLaCadenaNewRateAEcadaValor(t *testing.T) {
	in := scenarioEntradas(t, "150000", "0.80", nil, "none", 300)
	res, err := Scenario(in, scenarioValores(t, "1.50", "2", "2.30", "3"), fechaScenario)
	if err != nil {
		t.Fatalf("Scenario: %v", err)
	}
	if len(res.Values) != 4 {
		t.Fatalf("values = %d bloques, se esperaba 4", len(res.Values))
	}
	tins := []string{"2.30", "2.80", "3.10", "3.80"}
	ecos := []string{"1.50", "2", "2.30", "3"}
	for i, v := range res.Values {
		if !v.Assumed {
			t.Errorf("values[%d].assumed = false, se esperaba true", i)
		}
		if got := v.IndexValue.String(); got != ecos[i] {
			t.Errorf("values[%d].index_value = %q, se esperaba %q (ecoado tal cual)", i, got, ecos[i])
		}
		if got := v.AnnualRatePct.Decimal().StringFixed(2); got != tins[i] {
			t.Errorf("values[%d].annual_rate_pct = %s, se esperaba %s", i, got, tins[i])
		}
	}
	// La escala del eco llega intacta al JSON: "1.50" no puede ser "1.5".
	data, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"index_value":"1.50"`) {
		t.Errorf(`el JSON debe llevar el eco exacto "1.50":\n%s`, data)
	}
	// El bloque 2.30+0.80 = TIN 3.10 reproduce la cuota del caso conocido.
	if got := res.Values[2].Payment.Decimal().StringFixed(2); got != "719.14" {
		t.Errorf("values[2].payment = %s, se esperaba 719.14 (TIN 3.10)", got)
	}
	if res.Inputs.Spread.String() != "0.80" {
		t.Errorf("inputs.spread = %s, se esperaba 0.80", res.Inputs.Spread.String())
	}
	if res.Inputs.Floor != nil {
		t.Errorf("inputs.floor = %v, se esperaba ausente", res.Inputs.Floor)
	}
}

// Suelo y redondeo de escritura: el suelo corta antes del redondeo (orden
// normativo) y el redondeo al múltiplo declarado.
func TestScenarioSueloYRedondeoDeEscritura(t *testing.T) {
	t.Run("suelo", func(t *testing.T) {
		floor := mustAppDecimal(t, "2.50")
		in := scenarioEntradas(t, "150000", "0.80", &floor, "none", 300)
		res, err := Scenario(in, scenarioValores(t, "1.00"), fechaScenario)
		if err != nil {
			t.Fatalf("Scenario: %v", err)
		}
		if got := res.Values[0].AnnualRatePct.Decimal().StringFixed(2); got != "2.50" {
			t.Errorf("annual_rate_pct = %s, se esperaba 2.50 (suelo)", got)
		}
	})
	t.Run("redondeo 0.05", func(t *testing.T) {
		in := scenarioEntradas(t, "150000", "0", nil, "0.05", 300)
		res, err := Scenario(in, scenarioValores(t, "2.13"), fechaScenario)
		if err != nil {
			t.Fatalf("Scenario: %v", err)
		}
		if got := res.Values[0].AnnualRatePct.Decimal().StringFixed(2); got != "2.15" {
			t.Errorf("annual_rate_pct = %s, se esperaba 2.15 (redondeo 0.05)", got)
		}
	})
	t.Run("redondeo 1/8", func(t *testing.T) {
		in := scenarioEntradas(t, "150000", "0", nil, "1/8", 300)
		res, err := Scenario(in, scenarioValores(t, "2.10"), fechaScenario)
		if err != nil {
			t.Fatalf("Scenario: %v", err)
		}
		if got := res.Values[0].AnnualRatePct.Decimal().String(); got != "2.125" {
			t.Errorf("annual_rate_pct = %s, se esperaba 2.125 (redondeo 1/8)", got)
		}
	})
}

// La salida de scenario tampoco recomienda nada (P-08).
func TestScenarioSinLenguajeDeRecomendacion(t *testing.T) {
	in := scenarioEntradas(t, "150000", "0.80", nil, "none", 300)
	res, err := Scenario(in, scenarioValores(t, "1.50", "2", "2.5", "3"), fechaScenario)
	if err != nil {
		t.Fatalf("Scenario: %v", err)
	}
	sinLenguajeDeRecomendacion(t, res)
}

// Errores de uso: lista vacía, valor que no es decimal, redondeo no soportado
// y entradas del préstamo inválidas.
func TestScenarioErroresDeEntradaSonUsage(t *testing.T) {
	validas := scenarioValores(t, "2", "3")

	cases := []struct {
		name    string
		in      ScenarioInputs
		valores []model.Decimal
	}{
		{"lista vacía", scenarioEntradas(t, "150000", "0", nil, "none", 300), nil},
		{"redondeo no soportado", scenarioEntradas(t, "150000", "0", nil, "1/3", 300), validas},
		{"plazo cero", scenarioEntradas(t, "150000", "0", nil, "none", 0), validas},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Scenario(tc.in, tc.valores, fechaScenario)
			var e *errs.Error
			if !errors.As(err, &e) {
				t.Fatalf("el error debe llevar el sobre del contrato, got %v", err)
			}
			if e.Kind != errs.KindUsage {
				t.Errorf("kind = %q, se esperaba usage (mensaje: %s)", e.Kind, e.Message)
			}
			// El prefijo del paquete interno no es lenguaje de usuario.
			if strings.Contains(e.Message, "finance:") {
				t.Errorf("el mensaje no debe filtrar el prefijo interno \"finance:\": %s", e.Message)
			}
			if errs.ExitCode(e.Kind) != 2 {
				t.Errorf("código de salida = %d, se esperaba 2", errs.ExitCode(e.Kind))
			}
		})
	}
}
