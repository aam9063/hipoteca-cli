package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// `hipoteca scenario --index-values 1.50,2,2.30,3 --principal 150000 --months
// 300 --spread 0.8`: cada valor se devuelve tal cual, marcado assumed, y el
// valor 2.30 con el diferencial 0.8 da el TIN 3.10, cuya cuota sobre 150000 a
// 300 meses es 719.14 (caso de referencia 1).
func TestE2EScenarioDevuelveCadaValorTalCual(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	stdout, stderr, err := runRoot(t, deps, "scenario",
		"--index-values", "1.50,2,2.30,3", "--principal", "150000",
		"--months", "300", "--spread", "0.8")
	if err != nil {
		t.Fatalf("scenario debe tener éxito, got %v", err)
	}
	assertEmpty(t, "stderr", stderr)

	var res app.ScenarioResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("el stdout no es el JSON del contrato: %v\n%s", err, stdout)
	}
	if len(res.Values) != 4 {
		t.Fatalf("values = %d bloques, se esperaba 4", len(res.Values))
	}
	ecos := []string{"1.50", "2", "2.30", "3"}
	tins := []string{"2.30", "2.80", "3.10", "3.80"}
	for i, v := range res.Values {
		if !v.Assumed {
			t.Errorf("values[%d].assumed = false, se esperaba true (P-06)", i)
		}
		if v.IndexValue.String() != ecos[i] {
			t.Errorf("values[%d].index_value = %q, se esperaba %q (ecoado tal cual)", i, v.IndexValue.String(), ecos[i])
		}
		if got := v.AnnualRatePct.Decimal().StringFixed(2); got != tins[i] {
			t.Errorf("values[%d].annual_rate_pct = %s, se esperaba %s", i, got, tins[i])
		}
	}
	// El eco mantiene la escala en el JSON: "1.50" no puede serializarse "1.5".
	if !strings.Contains(stdout, `"index_value":"1.50"`) {
		t.Errorf("el JSON debe llevar el eco exacto \"1.50\":\n%s", stdout)
	}
	// TIN 3.10 → cuota 719.14 (caso de referencia 1).
	if got := res.Values[2].Payment.Decimal().StringFixed(2); got != "719.14" {
		t.Errorf("values[2].payment = %s, se esperaba 719.14", got)
	}
	if !res.Attribution.NotEndorsed {
		t.Error("not_endorsed debe ser true (P-05)")
	}
	assertSingleJSON(t, stdout)
}

// Faltan banderas, un valor no es decimal o el redondeo no existe: usage/2 con
// stdout vacío.
func TestE2EScenarioEntradasInvalidas(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	cases := [][]string{
		{"scenario"},
		{"scenario", "--principal", "150000", "--months", "300"},
		{"scenario", "--index-values", "1.5,2", "--months", "300"},
		{"scenario", "--index-values", "1.5,2", "--principal", "150000"},
		{"scenario", "--index-values", "", "--principal", "150000", "--months", "300"},
		{"scenario", "--index-values", "1.5,x", "--principal", "150000", "--months", "300"},
		{"scenario", "--index-values", "1.5,2", "--principal", "150000", "--months", "300", "--rounding", "1/3"},
		{"scenario", "--index-values", "1.5,2", "--principal", "150000", "--months", "0"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, _, err := runRoot(t, deps, args...)
			assertContractError(t, err, errs.KindUsage, 2)
			assertEmpty(t, "stdout", stdout)
		})
	}
}
