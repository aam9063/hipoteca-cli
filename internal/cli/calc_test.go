package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// `hipoteca calc --principal 150000 --rate 3.10 --months 300` reproduce el
// caso de referencia 1 al céntimo, sin préstamo guardado y sin red.
func TestE2ECalcReproduceElCasoDeReferencia(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	stdout, stderr, err := runRoot(t, deps, "calc", "--principal", "150000", "--rate", "3.10", "--months", "300")
	if err != nil {
		t.Fatalf("calc debe tener éxito, got %v", err)
	}
	assertEmpty(t, "stderr", stderr)

	var res app.CalcResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("el stdout no es el JSON del contrato: %v\n%s", err, stdout)
	}
	if got := res.Payment.Decimal().StringFixed(2); got != "719.14" {
		t.Errorf("payment = %s, se esperaba 719.14", got)
	}
	if got := res.TotalInterest.Decimal().StringFixed(2); got != "65743.56" {
		t.Errorf("total_interest = %s, se esperaba 65743.56", got)
	}
	if got := res.TotalPaid.Decimal().StringFixed(2); got != "215743.56" {
		t.Errorf("total_paid = %s, se esperaba 215743.56", got)
	}
	if res.Inputs.Principal.String() != "150000" || res.Inputs.AnnualRatePct.String() != "3.10" || res.Inputs.TermMonths != 300 {
		t.Errorf("inputs = %+v, se esperaba 150000 / 3.10 / 300", res.Inputs)
	}
	if !res.Attribution.NotEndorsed {
		t.Error("not_endorsed debe ser true (P-05)")
	}
	assertSingleJSON(t, stdout)
}

// Sin las tres banderas, o con un valor que no es decimal, es usage/2 con
// stdout vacío.
func TestE2ECalcEntradasInvalidas(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	cases := [][]string{
		{"calc"},
		{"calc", "--principal", "150000"},
		{"calc", "--principal", "150000", "--rate", "3.10"},
		{"calc", "--principal", "abc", "--rate", "3.10", "--months", "300"},
		{"calc", "--principal", "150000", "--rate", "x", "--months", "300"},
		{"calc", "--principal", "150000", "--rate", "3.10", "--months", "0"},
		{"calc", "--principal", "-5", "--rate", "3.10", "--months", "300"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, _, err := runRoot(t, deps, args...)
			assertContractError(t, err, errs.KindUsage, 2)
			assertEmpty(t, "stdout", stdout)
		})
	}
}
