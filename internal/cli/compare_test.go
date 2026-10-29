package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/loan"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// ----------------------------------------------------------------------------
// E2E de `compare` (FIN-04): el árbol cobra completo con una fuente de datos
// de prueba que reparte por código de serie, sin red. Se comprueba lo que ve
// el agente: un único valor JSON en stdout, los códigos del contrato y la
// salida comparativa sin recomendación ninguna (P-08, decisión 3 de FIN-04).
// ----------------------------------------------------------------------------

// mapSource sirve un bde.Series por código de serie: la fuente mínima para
// `compare`, que necesita los históricos de DOS índices.
type mapSource struct {
	series map[string]bde.Series
}

func (s mapSource) Latest(context.Context, []string) ([]bde.Observation, error) {
	return nil, nil
}

func (s mapSource) History(_ context.Context, id string, _ bde.Rango) (bde.Series, error) {
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		return bde.Series{}, err
	}
	idx, err := cat.Lookup(id)
	if err != nil {
		return bde.Series{}, err
	}
	sr, ok := s.series[idx.Code]
	if !ok {
		return bde.Series{}, errs.New(errs.KindIndexNotPublished,
			"el índice "+id+" no tiene histórico en la fuente de prueba", "")
	}
	return sr, nil
}

// serieDe construye un histórico ascendente de prueba con los meses dados.
func serieDe(code string, meses map[string]string) bde.Series {
	obs := make([]bde.Observation, 0, len(meses))
	for mes, valor := range meses {
		m, _ := time.Parse("2006-01", mes)
		v, err := model.NewDecimal(valor)
		if err != nil {
			panic(err)
		}
		obs = append(obs, bde.Observation{
			Code:  code,
			Month: time.Date(m.Year(), m.Month(), 1, 0, 0, 0, 0, time.UTC),
			Value: v,
		})
	}
	return bde.Series{Code: code, End: time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC), Observations: obs}
}

func TestE2ECompareComparaSinRecomendar(t *testing.T) {
	src := mapSource{series: map[string]bde.Series{
		"D_1NBAF472": serieDe("D_1NBAF472", map[string]string{
			"2024-11": "2.506", "2025-11": "2.217", "2026-09": "3.247",
		}),
		"D_1T9H0000": serieDe("D_1T9H0000", map[string]string{
			"2024-11": "3.206", "2025-11": "2.807", "2026-09": "3.189",
		}),
	}}
	deps := depsConConfig(t, src, func(path string) {
		initialPeriod := 12
		offset := 2
		sinSuelo := ""
		l := loan.Loan{
			Name: "Casa", Type: loan.TypeVariable, StartDate: "2024-01-01",
			OriginalPrincipal: "150000.00", TermMonths: 36, PaymentDay: 1,
			InitialRate: "2.00", InitialPeriodMonths: &initialPeriod,
			Index: "euribor-1y", Spread: "0.50", RevisionEveryMonths: 12,
			ReferenceOffsetMonths: &offset, FloorRate: &sinSuelo,
			RateRounding: "none", InterestConvention: loan.InterestConventionMonthlyNominal,
			PaymentRounding: loan.PaymentRoundingHalfUpCents, EarlyRepaymentFeePct: "0",
		}
		if err := app.LoanAdd(path, "casa", l); err != nil {
			t.Fatalf("setup: loan add: %v", err)
		}
	})

	stdout, stderr, err := runRoot(t, deps, "compare", "casa", "--index", "irph", "--spread", "1.00")
	if err != nil {
		t.Fatalf("compare debe tener éxito, got %v (stderr: %s)", err, stderr)
	}
	assertEmpty(t, "stderr", stderr)
	assertSingleJSON(t, stdout)

	var res app.CompareResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("el stdout no es el JSON del contrato: %v\n%s", err, stdout)
	}
	if res.Own.Index.ID != "euribor-1y" || res.Alternative.Index.ID != "irph" {
		t.Fatalf("los dos índices deben declararse: %q / %q", res.Own.Index.ID, res.Alternative.Index.ID)
	}
	if len(res.Own.Revisions) != 2 || len(res.Alternative.Revisions) != 2 {
		t.Fatalf("dos revisiones en cada bloque: %d / %d", len(res.Own.Revisions), len(res.Alternative.Revisions))
	}
	if res.Own.Revisions[0].IndexValue.String() != "2.506" || res.Alternative.Revisions[0].IndexValue.String() != "3.206" {
		t.Fatalf("cada revisión usa el índice de su mes: %s / %s",
			res.Own.Revisions[0].IndexValue, res.Alternative.Revisions[0].IndexValue)
	}

	// P-08 extendido (decisión 3 de FIN-04): ninguna palabra de
	// asesoramiento, ni en español ni en inglés.
	lower := strings.ToLower(stdout)
	for _, palabra := range []string{"conviene", "mejor", "recomend", "deberías", "ganador", "winner"} {
		if strings.Contains(lower, palabra) {
			t.Errorf("la salida contiene la palabra prohibida %q:\n%s", palabra, stdout)
		}
	}
}

// Las banderas mal puestas son usage/2 (§6.4) con el stdout vacío (P-01).
// El --to posterior a los meses publicados también: la comparación sólo
// cubre meses publicados (FIN-04, cierre).
func TestE2ECompareFlagIncorrecta(t *testing.T) {
	src := mapSource{series: map[string]bde.Series{
		"D_1NBAF472": serieDe("D_1NBAF472", map[string]string{
			"2024-11": "2.506", "2025-11": "2.217", "2026-09": "3.247",
		}),
		"D_1T9H0000": serieDe("D_1T9H0000", map[string]string{
			"2024-11": "3.206", "2025-11": "2.807", "2026-09": "3.189",
		}),
	}}
	deps := depsConConfig(t, src, func(path string) {
		initialPeriod := 12
		offset := 2
		sinSuelo := ""
		l := loan.Loan{
			Name: "Casa", Type: loan.TypeVariable, StartDate: "2024-01-01",
			OriginalPrincipal: "150000.00", TermMonths: 36, PaymentDay: 1,
			InitialRate: "2.00", InitialPeriodMonths: &initialPeriod,
			Index: "euribor-1y", Spread: "0.50", RevisionEveryMonths: 12,
			ReferenceOffsetMonths: &offset, FloorRate: &sinSuelo,
			RateRounding: "none", InterestConvention: loan.InterestConventionMonthlyNominal,
			PaymentRounding: loan.PaymentRoundingHalfUpCents, EarlyRepaymentFeePct: "0",
		}
		if err := app.LoanAdd(path, "casa", l); err != nil {
			t.Fatalf("setup: loan add: %v", err)
		}
	})

	cases := []struct {
		name string
		args []string
	}{
		{"sin índice alternativo", []string{"compare", "casa"}},
		{"sin diferencial", []string{"compare", "casa", "--index", "irph"}},
		{"diferencial no decimal", []string{"compare", "casa", "--index", "irph", "--spread", "abc"}},
		{"ventana mal formada", []string{"compare", "casa", "--index", "irph", "--spread", "1.00", "--from", "2026"}},
		{"--to posterior a los meses publicados", []string{"compare", "casa", "--index", "irph", "--spread", "1.00", "--to", "2027-01"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, err := runRoot(t, deps, tc.args...)
			assertContractError(t, err, errs.KindUsage, 2)
			assertEmpty(t, "stdout", stdout)
		})
	}
}
