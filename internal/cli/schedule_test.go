package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// ----------------------------------------------------------------------------
// E2E de `revision` y `schedule` sobre un préstamo guardado, con la serie del
// índice inyectada (sin red). Los valores son los del fixture real
// testdata/bde/listaSeries-euribor-1y-2026.json (Fuente: Banco de España);
// los tests contra el servidor de fixtures están en internal/app.
// ----------------------------------------------------------------------------

// serieFixture devuelve la serie euribor-1y del fixture de 2026: meses
// 2026-01..2026-09 (termina en 2026-09 con valor 3.247), con los metadatos
// de publicación del fixture.
func serieFixture(t *testing.T) stubSource {
	t.Helper()
	meses := []struct {
		mes   string
		valor string
	}{
		{"2026-01", "2.245"},
		{"2026-02", "2.221"},
		{"2026-03", "2.565"},
		{"2026-04", "2.747"},
		{"2026-05", "2.804"},
		{"2026-06", "2.798"},
		{"2026-07", "2.855"},
		{"2026-08", "2.954"},
		{"2026-09", "3.247"},
	}
	obs := make([]bde.Observation, 0, len(meses))
	for _, m := range meses {
		mes, err := time.Parse("2006-01", m.mes)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		valor, err := model.NewDecimal(m.valor)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		obs = append(obs, bde.Observation{
			Code:  "D_1NBAF472",
			Month: mes,
			Value: valor,
		})
	}
	fin, _ := time.Parse("2006-01", "2026-09")
	return stubSource{series: bde.Series{
		Code:             "D_1NBAF472",
		Description:      "Euribor a un año",
		DescriptionShort: "Euribor a un año",
		Frequency:        "M",
		Decimals:         3,
		Start:            fin.AddDate(-27, 0, 0),
		End:              fin,
		Observations:     obs,
	}}
}

// TestE2ERevisionPublicada: add → revision --date 2026-11 sobre un préstamo
// con recibo: un único valor JSON con el índice publicado de 2026-09, la
// cadena del tipo y las cuotas antes/después.
func TestE2ERevisionPublicada(t *testing.T) {
	src := serieFixture(t)
	deps := depsConConfig(t, &src, seedLoan(t, "casa"))

	stdout, stderr, err := runRoot(t, deps, "revision", "casa", "--date", "2026-11")
	if err != nil {
		t.Fatalf("revision: %v (stderr: %s)", err, stderr)
	}
	assertSingleJSON(t, stdout)

	var res struct {
		SchemaVersion int    `json:"schema_version"`
		Loan          string `json:"loan"`
		RevisionDate  string `json:"revision_date"`
		Index         struct {
			ReferenceMonth string `json:"reference_month"`
			Value          string `json:"value"`
			Status         string `json:"status"`
			Series         string `json:"series"`
		} `json:"index"`
		Rate struct {
			Previous string `json:"previous"`
			Spread   string `json:"spread"`
			Raw      string `json:"raw"`
			New      string `json:"new"`
		} `json:"rate"`
		Inputs struct {
			OutstandingPrincipal string `json:"outstanding_principal"`
			RemainingMonths      int    `json:"remaining_months"`
			OutstandingSource    string `json:"outstanding_source"`
		} `json:"inputs"`
		Payment struct {
			Previous string `json:"previous"`
			New      string `json:"new"`
		} `json:"payment"`
		Warnings    []string `json:"warnings"`
		Attribution struct {
			Text          string `json:"text"`
			DataUpdatedAt string `json:"data_updated_at"`
			NotEndorsed   bool   `json:"not_endorsed"`
		} `json:"attribution"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("la salida no es JSON válido: %v\n%s", err, stdout)
	}
	if res.SchemaVersion != 1 || res.Loan != "casa" || res.RevisionDate != "2026-11-01" {
		t.Fatalf("cabecera = %+v", res)
	}
	if res.Index.ReferenceMonth != "2026-09" || res.Index.Value != "3.247" ||
		res.Index.Status != "published" || res.Index.Series != "D_1NBAF472" {
		t.Fatalf("index = %+v", res.Index)
	}
	if res.Rate.Previous != "3.12" || res.Rate.Spread != "0.89" || res.Rate.Raw != "4.137" || res.Rate.New != "4.137" {
		t.Fatalf("rate = %+v", res.Rate)
	}
	if res.Inputs.OutstandingSource != "sync+schedule" || res.Inputs.RemainingMonths != 295 ||
		res.Inputs.OutstandingPrincipal != "152341.18" {
		t.Fatalf("inputs = %+v", res.Inputs)
	}
	if res.Payment.Previous == "" || res.Payment.New == "" || res.Payment.Previous == res.Payment.New {
		t.Fatalf("payment = %+v", res.Payment)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want vacío", res.Warnings)
	}
	if !res.Attribution.NotEndorsed ||
		!strings.Contains(res.Attribution.Text, "Banco de España") {
		t.Fatalf("attribution = %+v", res.Attribution)
	}
}

// TestE2ERevisionAssumed: con --assume el índice va marcado assumed y la
// salida avisa (P-06); no se consulta la serie.
func TestE2ERevisionAssumed(t *testing.T) {
	deps := depsConConfig(t, &stubSource{}, seedLoan(t, "casa"))

	stdout, _, err := runRoot(t, deps, "revision", "casa", "--date", "2026-11", "--assume", "2.35")
	if err != nil {
		t.Fatalf("revision --assume: %v", err)
	}
	assertSingleJSON(t, stdout)
	var res struct {
		Index struct {
			Value  string `json:"value"`
			Status string `json:"status"`
		} `json:"index"`
		Warnings []string `json:"warnings"`
		Rate     struct {
			Raw string `json:"raw"`
		} `json:"rate"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("la salida no es JSON válido: %v", err)
	}
	if res.Index.Status != "assumed" || res.Index.Value != "2.35" {
		t.Fatalf("index = %+v, want assumed/2.35", res.Index)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings = %v, want el aviso del supuesto", res.Warnings)
	}
	if res.Rate.Raw != "3.24" {
		t.Fatalf("rate.raw = %s, want 3.24 (2.35 + 0.89)", res.Rate.Raw)
	}
}

// TestE2ERevisionMesNoPublicado: un mes de referencia sin valor en la serie
// es index_not_published/4 con el hint de --assume (P-06).
func TestE2ERevisionMesNoPublicado(t *testing.T) {
	src := serieFixture(t)
	deps := depsConConfig(t, &src, seedLoan(t, "casa"))

	stdout, _, err := runRoot(t, deps, "revision", "casa", "--date", "2026-12")
	e := assertContractError(t, err, errs.KindIndexNotPublished, 4)
	assertEmpty(t, "stdout", stdout)
	if !strings.Contains(e.Hint, "--assume") || !strings.Contains(e.Message, "2026-10") {
		t.Fatalf("error = %+v", e)
	}
}

// TestE2EScheduleDesdePrestamo: schedule con recibo: el cuadro parte del
// recibo (296 cuotas, primera el mes siguiente al recibo) y --from/--to
// recortan; con un préstamo incompleto responde loan_incomplete/2.
func TestE2EScheduleDesdePrestamo(t *testing.T) {
	src := serieFixture(t)
	deps := depsConConfig(t, &src, seedLoan(t, "casa"))

	stdout, _, err := runRoot(t, deps, "schedule", "casa")
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	assertSingleJSON(t, stdout)
	var res struct {
		Loan   string `json:"loan"`
		Months int    `json:"months"`
		Rows   []struct {
			Month int    `json:"month"`
			Date  string `json:"date"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("la salida no es JSON válido: %v", err)
	}
	if res.Loan != "casa" || res.Months != 296 || len(res.Rows) != 296 {
		t.Fatalf("cabecera del cuadro = %+v", res)
	}
	if res.Rows[0].Date != "2026-11-01" {
		t.Fatalf("primera cuota = %s, want 2026-11-01", res.Rows[0].Date)
	}

	// Recorte --from/--to.
	stdout, _, err = runRoot(t, deps, "schedule", "casa", "--from", "2027-01", "--to", "2027-02")
	if err != nil {
		t.Fatalf("schedule con recorte: %v", err)
	}
	var recorte struct {
		Rows []struct {
			Date string `json:"date"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(stdout), &recorte); err != nil {
		t.Fatalf("la salida no es JSON válido: %v", err)
	}
	if len(recorte.Rows) != 2 || recorte.Rows[0].Date != "2027-01-01" || recorte.Rows[1].Date != "2027-02-01" {
		t.Fatalf("recorte = %+v", recorte)
	}

	// schedule sin id y un único préstamo: opcional.
	if _, _, err := runRoot(t, deps, "schedule"); err != nil {
		t.Fatalf("schedule sin id y un préstamo: %v", err)
	}
}
