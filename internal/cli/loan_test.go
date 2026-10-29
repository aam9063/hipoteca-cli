package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/loan"
)

// ----------------------------------------------------------------------------
// E2E del grupo `loan` sobre el árbol cobra completo con una config temporal
// (LOAN-01.2): cada comando emite un único valor JSON en stdout (P-01) y los
// códigos de salida son los del contrato (§6.4).
// ----------------------------------------------------------------------------

// depsConConfig construye el árbol con la fuente dada y una config temporal;
// el seed opcional la escribe antes de ejecutar nada.
func depsConConfig(t *testing.T, src app.Source, seed func(path string)) Deps {
	t.Helper()
	deps := depsCon(t, src)
	path := filepath.Join(t.TempDir(), "config.toml")
	if seed != nil {
		seed(path)
	}
	deps.ConfigPath = func() (string, error) {
		return path, nil
	}
	return deps
}

// flagsCasa son las banderas de un préstamo completo del ejemplo del
// arranque, con el recibo sincronizado en 2026-10.
func flagsCasa() []string {
	return []string{
		"--name", "Casa", "--type", "variable", "--start-date", "2021-06-01",
		"--principal", "180000", "--months", "360", "--payment-day", "1",
		"--initial-rate", "1.25", "--initial-period-months", "12",
		"--index", "euribor-1y", "--spread", "0.89",
		"--revision-every-months", "12", "--reference-offset-months", "2",
		"--floor", "none", "--rate-rounding", "none", "--fee-pct", "0",
	}
}

// TestE2EAddShowListRemove: el ciclo completo del préstamo. Cada comando
// emite un único valor JSON; remove sin --yes responde confirmation_required
// y deja el préstamo intacto (decisión 3 de LOAN-01).
func TestE2EAddShowListRemove(t *testing.T) {
	deps := depsConConfig(t, &stubSource{}, nil)

	// add: guarda y muestra el préstamo tal cual quedó.
	stdout, stderr, err := runRoot(t, deps, append([]string{"loan", "add", "casa"}, flagsCasa()...)...)
	if err != nil {
		t.Fatalf("loan add: %v (stderr: %s)", err, stderr)
	}
	assertSingleJSON(t, stdout)
	if !strings.Contains(stdout, `"casa"`) || !strings.Contains(stdout, `"complete":true`) {
		t.Fatalf("la salida de add debe mostrar el préstamo completo:\n%s", stdout)
	}

	// show: los campos del préstamo, con el suelo "sin suelo" explícito.
	stdout, _, err = runRoot(t, deps, "loan", "show", "casa")
	if err != nil {
		t.Fatalf("loan show: %v", err)
	}
	assertSingleJSON(t, stdout)
	var show struct {
		Loan struct {
			ID                    string  `json:"id"`
			OriginalPrincipal     string  `json:"original_principal"`
			TermMonths            int     `json:"term_months"`
			ReferenceOffsetMonths *int    `json:"reference_offset_months"`
			FloorRate             *string `json:"floor_rate"`
		} `json:"loan"`
	}
	if err := json.Unmarshal([]byte(stdout), &show); err != nil {
		t.Fatalf("la salida de show no es JSON válido: %v\n%s", err, stdout)
	}
	if show.Loan.ID != "casa" || show.Loan.OriginalPrincipal != "180000" ||
		show.Loan.TermMonths != 360 || show.Loan.ReferenceOffsetMonths == nil ||
		*show.Loan.ReferenceOffsetMonths != 2 || show.Loan.FloorRate == nil || *show.Loan.FloorRate != "" {
		t.Fatalf("show devolvió: %s", stdout)
	}

	// show sin id con un único préstamo: opcional (LOAN-01.4).
	stdout, _, err = runRoot(t, deps, "loan", "show")
	if err != nil {
		t.Fatalf("loan show sin id y un préstamo: %v", err)
	}
	if !strings.Contains(stdout, `"casa"`) {
		t.Fatalf("show sin id debía resolver el único préstamo: %s", stdout)
	}

	// list: una fila con la marca de completo.
	stdout, _, err = runRoot(t, deps, "loan", "list")
	if err != nil {
		t.Fatalf("loan list: %v", err)
	}
	assertSingleJSON(t, stdout)
	if !strings.Contains(stdout, `"complete":true`) {
		t.Fatalf("list debe marcar el préstamo como completo: %s", stdout)
	}

	// remove sin --yes: confirmation_required/2 y el préstamo sigue ahí.
	stdout, _, err = runRoot(t, deps, "loan", "remove", "casa")
	e := assertContractError(t, err, errs.KindConfirmationRequired, 2)
	assertEmpty(t, "stdout", stdout)
	if !strings.Contains(e.Hint, "--yes") {
		t.Fatalf("el hint debe apuntar a --yes: %q", e.Hint)
	}
	if _, _, err = runRoot(t, deps, "loan", "show", "casa"); err != nil {
		t.Fatalf("el préstamo debía seguir guardado tras el rechazo: %v", err)
	}

	// remove --yes: borra y el show posterior es not_found/4.
	stdout, _, err = runRoot(t, deps, "loan", "remove", "casa", "--yes")
	if err != nil {
		t.Fatalf("loan remove --yes: %v", err)
	}
	assertSingleJSON(t, stdout)
	_, _, err = runRoot(t, deps, "loan", "show", "casa")
	assertContractError(t, err, errs.KindNotFound, 4)
}

// TestE2ESyncYEcho: sync guarda los tres datos del recibo y la salida los
// muestra; faltan banderas → usage/2.
func TestE2ESyncYEcho(t *testing.T) {
	deps := depsConConfig(t, &stubSource{}, nil)
	if _, _, err := runRoot(t, deps, append([]string{"loan", "add", "casa"}, flagsCasa()...)...); err != nil {
		t.Fatalf("loan add: %v", err)
	}

	stdout, _, err := runRoot(t, deps, "loan", "sync", "casa",
		"--date", "2026-10-01", "--outstanding", "152341.18", "--rate", "3.12")
	if err != nil {
		t.Fatalf("loan sync: %v", err)
	}
	assertSingleJSON(t, stdout)
	var show struct {
		Loan struct {
			Sync *struct {
				Date                 string `json:"date"`
				OutstandingPrincipal string `json:"outstanding_principal"`
				CurrentRate          string `json:"current_rate"`
			} `json:"sync"`
		} `json:"loan"`
	}
	if err := json.Unmarshal([]byte(stdout), &show); err != nil {
		t.Fatalf("la salida de sync no es JSON válido: %v", err)
	}
	if show.Loan.Sync == nil || show.Loan.Sync.Date != "2026-10-01" ||
		show.Loan.Sync.OutstandingPrincipal != "152341.18" || show.Loan.Sync.CurrentRate != "3.12" {
		t.Fatalf("sync devolvió: %s", stdout)
	}

	// Faltan banderas: usage/2, y el sync anterior sigue guardado.
	_, _, err = runRoot(t, deps, "loan", "sync", "casa", "--date", "2026-11-01")
	assertContractError(t, err, errs.KindUsage, 2)
	stdout, _, err = runRoot(t, deps, "loan", "show", "casa")
	if err != nil || !strings.Contains(stdout, "2026-10-01") {
		t.Fatalf("el sync válido debía seguir guardado: %s (%v)", stdout, err)
	}
}

// TestE2EAddDuplicadoYListVacia: el id es único y una instalación nueva
// lista una lista vacía (un único valor JSON, no un error).
func TestE2EAddDuplicadoYListVacia(t *testing.T) {
	deps := depsConConfig(t, &stubSource{}, nil)

	stdout, _, err := runRoot(t, deps, "loan", "list")
	if err != nil {
		t.Fatalf("loan list sin préstamos: %v", err)
	}
	assertSingleJSON(t, stdout)

	if _, _, err := runRoot(t, deps, append([]string{"loan", "add", "casa"}, flagsCasa()...)...); err != nil {
		t.Fatalf("loan add: %v", err)
	}
	_, _, err = runRoot(t, deps, append([]string{"loan", "add", "casa"}, flagsCasa()...)...)
	assertContractError(t, err, errs.KindUsage, 2)

	// Un id desconocido en cualquier comando de préstamo es not_found/4.
	_, _, err = runRoot(t, deps, "loan", "show", "piso")
	assertContractError(t, err, errs.KindNotFound, 4)
}

// TestE2EVariosPrestamosSinId: con dos préstamos guardados y sin id, los
// comandos con id opcional responden usage/2 listando los ids (LOAN-01.4).
func TestE2EVariosPrestamosSinId(t *testing.T) {
	deps := depsConConfig(t, &stubSource{}, nil)
	for _, id := range []string{"casa", "piso"} {
		if _, _, err := runRoot(t, deps, append([]string{"loan", "add", id}, flagsCasa()...)...); err != nil {
			t.Fatalf("loan add %s: %v", id, err)
		}
	}
	for _, cmd := range [][]string{{"loan", "show"}, {"loan", "sync"}, {"revision"}, {"schedule"}, {"amortize", "--amount", "10000", "--date", "60", "--mode", "compare"}} {
		_, _, err := runRoot(t, deps, cmd...)
		e := assertContractError(t, err, errs.KindUsage, 2)
		if !strings.Contains(e.Message, "casa") || !strings.Contains(e.Message, "piso") {
			t.Fatalf("el error debe listar los ids: %q", e.Message)
		}
	}
}

// seedLoan escribe un préstamo completo con recibo directamente en la config
// temporal (para los tests que no pasan por `loan add`).
func seedLoan(t *testing.T, id string) func(path string) {
	t.Helper()
	return func(path string) {
		cfg := config.Default()
		cfg.Loans = map[string]loan.Loan{id: loanCompletaParaE2E()}
		if err := cfg.Save(path); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
}

// loanCompletaParaE2E es el préstamo del ejemplo del arranque con recibo.
func loanCompletaParaE2E() loan.Loan {
	initialPeriod := 12
	offset := 2
	sinSuelo := ""
	return loan.Loan{
		Name:                  "Casa",
		Type:                  loan.TypeVariable,
		StartDate:             "2021-06-01",
		OriginalPrincipal:     "180000.00",
		TermMonths:            360,
		PaymentDay:            1,
		InitialRate:           "1.25",
		InitialPeriodMonths:   &initialPeriod,
		Index:                 "euribor-1y",
		Spread:                "0.89",
		RevisionEveryMonths:   12,
		ReferenceOffsetMonths: &offset,
		FloorRate:             &sinSuelo,
		RateRounding:          "none",
		InterestConvention:    loan.InterestConventionMonthlyNominal,
		PaymentRounding:       loan.PaymentRoundingHalfUpCents,
		EarlyRepaymentFeePct:  "0",
		Sync:                  &loan.Sync{Date: "2026-10-01", OutstandingPrincipal: "152341.18", CurrentRate: "3.12"},
	}
}

// TestE2EEditCambiaUnCampo: `loan edit` cambia sólo la bandera que llega
// sobre el préstamo ya guardado y valida el préstamo FUSIONADO: con un
// préstamo completo, editar un campo sin repetir el resto funciona (el hint
// de loan_incomplete lo anuncia así) y el cambio queda en la config. Una
// bandera inválida sobre el resultado fusionado sigue fallando con usage/2
// nombrando el campo, sin tocar la config.
func TestE2EEditCambiaUnCampo(t *testing.T) {
	deps := depsConConfig(t, &stubSource{}, seedLoan(t, "casa"))

	stdout, stderr, err := runRoot(t, deps, "loan", "edit", "casa", "--reference-offset-months", "4")
	if err != nil {
		t.Fatalf("loan edit de un campo sobre un préstamo completo: %v (stderr: %s)", err, stderr)
	}
	assertSingleJSON(t, stdout)
	if !strings.Contains(stdout, `"reference_offset_months":4`) {
		t.Fatalf("la salida de edit debe reflejar el campo cambiado:\n%s", stdout)
	}
	// El cambio queda en la config y el resto del préstamo no se toca.
	stdout, _, err = runRoot(t, deps, "loan", "show", "casa")
	if err != nil || !strings.Contains(stdout, `"reference_offset_months":4`) ||
		!strings.Contains(stdout, `"spread":"0.89"`) {
		t.Fatalf("el cambio debía quedar en la config: %s (%v)", stdout, err)
	}

	// Una bandera inválida: usage/2 nombrando el campo, y la config se
	// queda como estaba.
	_, _, err = runRoot(t, deps, "loan", "edit", "casa", "--type", "flotante")
	e := assertContractError(t, err, errs.KindUsage, 2)
	if !strings.Contains(e.Message, "flotante") {
		t.Fatalf("el error debe nombrar el campo y el valor: %q", e.Message)
	}
	stdout, _, err = runRoot(t, deps, "loan", "show", "casa")
	if err != nil || !strings.Contains(stdout, `"type":"variable"`) {
		t.Fatalf("una edición rechazada no debe cambiar la config: %s (%v)", stdout, err)
	}
}

// TestE2ERemoveIdOpcional: con un único préstamo, `loan remove --yes` sin id
// borra ése (LOAN-01.4); con varios préstamos y sin id, usage/2 listando los
// ids. Sin --yes nada se borra (decisión 3 de LOAN-01).
func TestE2ERemoveIdOpcional(t *testing.T) {
	deps := depsConConfig(t, &stubSource{}, seedLoan(t, "casa"))

	stdout, _, err := runRoot(t, deps, "loan", "remove", "--yes")
	if err != nil {
		t.Fatalf("loan remove --yes sin id y un préstamo: %v", err)
	}
	assertSingleJSON(t, stdout)
	if !strings.Contains(stdout, `"loan":"casa"`) || !strings.Contains(stdout, `"removed":true`) {
		t.Fatalf("remove debía borrar casa: %s", stdout)
	}
	_, _, err = runRoot(t, deps, "loan", "show")
	assertContractError(t, err, errs.KindNotFound, 4)

	for _, id := range []string{"casa", "piso"} {
		if _, _, err := runRoot(t, deps, append([]string{"loan", "add", id}, flagsCasa()...)...); err != nil {
			t.Fatalf("loan add %s: %v", id, err)
		}
	}
	_, _, err = runRoot(t, deps, "loan", "remove", "--yes")
	e := assertContractError(t, err, errs.KindUsage, 2)
	if !strings.Contains(e.Message, "casa") || !strings.Contains(e.Message, "piso") {
		t.Fatalf("el error debe listar los ids: %q", e.Message)
	}
}

// assertSingleJSON ya está definido en index_test.go (mismo paquete): un
// único valor JSON terminado en salto de línea.
