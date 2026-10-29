package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/loan"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/shopspring/decimal"
)

// ----------------------------------------------------------------------------
// Operaciones `loan` sobre la config local (LOAN-01.2 y LOAN-01.3). Cada
// prueba trabaja sobre una config temporal escrita con el mecanismo de
// CORE-02: nada toca la config del usuario.
// ----------------------------------------------------------------------------

// prestamoCompleto devuelve un préstamo completo para las pruebas de la capa
// de aplicación.
func prestamoCompleto() loan.Loan {
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
	}
}

// configCon devuelve la ruta de una config temporal con los préstamos dados,
// escrita con config.Save (escritura atómica de CORE-02).
func configCon(t *testing.T, loans map[string]loan.Loan) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	cfg := config.Default()
	cfg.Loans = loans
	if err := cfg.Save(path); err != nil {
		t.Fatalf("setup: Save(): %v", err)
	}
	return path
}

// TestLoanAddYShowIdaYVuelta: lo que se guarda es lo que se muestra, campo a
// campo, incluidos floor_rate = "" y los punteros.
func TestLoanAddYShowIdaYVuelta(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	want := prestamoCompleto()

	if err := LoanAdd(path, "casa", want); err != nil {
		t.Fatalf("LoanAdd(): %v", err)
	}
	res, err := LoanShow(path, "casa")
	if err != nil {
		t.Fatalf("LoanShow(): %v", err)
	}
	if res.SchemaVersion != model.SchemaVersion {
		t.Fatalf("schema_version = %d, want %d", res.SchemaVersion, model.SchemaVersion)
	}
	if res.Loan.ID != "casa" || !res.Loan.Complete {
		t.Fatalf("view = id %q complete %v, want casa/true", res.Loan.ID, res.Loan.Complete)
	}
	if len(res.Loan.MissingFields) != 0 {
		t.Fatalf("missing_fields = %v, want vacío", res.Loan.MissingFields)
	}
	got := res.Loan.Loan
	if got.Name != want.Name || got.Type != want.Type || got.StartDate != want.StartDate ||
		got.OriginalPrincipal != want.OriginalPrincipal || got.TermMonths != want.TermMonths ||
		got.PaymentDay != want.PaymentDay || got.InitialRate != want.InitialRate ||
		got.Index != want.Index || got.Spread != want.Spread ||
		got.RevisionEveryMonths != want.RevisionEveryMonths || got.RateRounding != want.RateRounding ||
		got.EarlyRepaymentFeePct != want.EarlyRepaymentFeePct {
		t.Fatalf("el préstamo mostrado no es el guardado:\nwant %+v\ngot  %+v", want, got)
	}
	if got.FloorRate == nil || *got.FloorRate != "" {
		t.Fatalf("floor_rate = %v, want \"\" (sin suelo explícito)", got.FloorRate)
	}
	if got.ReferenceOffsetMonths == nil || *got.ReferenceOffsetMonths != 2 {
		t.Fatalf("reference_offset_months = %v, want 2", got.ReferenceOffsetMonths)
	}
	if got.Sync != nil {
		t.Fatalf("sync = %v, want ausente", got.Sync)
	}
}

// TestLoanAddDuplicadoYVacio: el id es obligatorio y único.
func TestLoanAddDuplicadoYVacio(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoCompleto()})

	if err := LoanAdd(path, "casa", prestamoCompleto()); err == nil {
		t.Fatal("LoanAdd() sobre un id existente: error = nil, want usage")
	}
	err := LoanAdd(path, "", prestamoCompleto())
	e := assertAppError(t, err, errs.KindUsage, 2)
	if e.Message == "" {
		t.Fatal("el error de id vacío debe llevar mensaje")
	}
}

// TestLoanAddValorInvalido: un valor presente pero imposible no se guarda.
func TestLoanAddValorInvalido(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	l := prestamoCompleto()
	l.OriginalPrincipal = "mil"
	if err := LoanAdd(path, "casa", l); err == nil {
		t.Fatal("LoanAdd() con capital inválido: error = nil, want usage")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("una incorporación rechazada no debe escribir la config")
	}
}

// TestLoanList: lista ordenada por id con la marca de completo, y una
// instalación sin préstamos devuelve una lista vacía (no es error).
func TestLoanList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	vacia, err := LoanList(path)
	if err != nil {
		t.Fatalf("LoanList() sin config: %v", err)
	}
	if len(vacia.Loans) != 0 {
		t.Fatalf("LoanList() sin préstamos = %v, want vacía", vacia.Loans)
	}

	incompleto := prestamoCompleto()
	incompleto.FloorRate = nil
	path = configCon(t, map[string]loan.Loan{
		"zulo":  prestamoCompleto(),
		"piso":  incompleto,
		"atabo": prestamoCompleto(),
	})
	res, err := LoanList(path)
	if err != nil {
		t.Fatalf("LoanList(): %v", err)
	}
	if len(res.Loans) != 3 {
		t.Fatalf("LoanList() = %d préstamos, want 3", len(res.Loans))
	}
	if res.Loans[0].ID != "atabo" || res.Loans[1].ID != "piso" || res.Loans[2].ID != "zulo" {
		t.Fatalf("LoanList() desordenado: %v", res.Loans)
	}
	if !res.Loans[2].Complete || res.Loans[1].Complete {
		t.Fatalf("complete mal marcado: %+v", res.Loans)
	}
}

// TestLoanShowIncompletoAvisa: show de un préstamo incompleto no falla:
// lista los campos que faltan y avisa por cada uno (arranque §4.1).
func TestLoanShowIncompletoAvisa(t *testing.T) {
	incompleto := prestamoCompleto()
	incompleto.FloorRate = nil
	incompleto.ReferenceOffsetMonths = nil
	path := configCon(t, map[string]loan.Loan{"casa": incompleto})

	res, err := LoanShow(path, "casa")
	if err != nil {
		t.Fatalf("LoanShow() de un incompleto: %v", err)
	}
	if res.Loan.Complete {
		t.Fatal("complete = true, want false")
	}
	if len(res.Loan.MissingFields) != 2 {
		t.Fatalf("missing_fields = %v, want [floor_rate reference_offset_months]", res.Loan.MissingFields)
	}
	if len(res.Loan.Warnings) != 2 {
		t.Fatalf("warnings = %v, want un aviso por campo", res.Loan.Warnings)
	}
}

// TestLoanShowIdOpcionalYErrores: con un único préstamo, omitir el id es
// válido (LOAN-01.4); con varios es usage/2 listando los ids; con un id
// desconocido, not_found/4.
func TestLoanShowIdOpcionalYErrores(t *testing.T) {
	solo := configCon(t, map[string]loan.Loan{"casa": prestamoCompleto()})
	res, err := LoanShow(solo, "")
	if err != nil {
		t.Fatalf("LoanShow() sin id y un préstamo: %v", err)
	}
	if res.Loan.ID != "casa" {
		t.Fatalf("resolvió %q, want casa", res.Loan.ID)
	}

	varios := configCon(t, map[string]loan.Loan{
		"casa": prestamoCompleto(),
		"piso": prestamoCompleto(),
	})
	_, err = LoanShow(varios, "")
	e := assertAppError(t, err, errs.KindUsage, 2)
	if e.Message == "" || !containsAll(e.Message, "casa", "piso") {
		t.Fatalf("el error de varios préstamos debe listar los ids: %q", e.Message)
	}
	_, err = LoanShow(varios, "atico")
	assertAppError(t, err, errs.KindNotFound, 4)
}

// TestLoanEditAplicaYCambia: edit aplica sólo los campos tocados y guarda.
func TestLoanEditAplicaYCambia(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoCompleto()})

	nuevoSpread := "1.10"
	err := LoanEdit(path, "casa", func(l *loan.Loan) { l.Spread = nuevoSpread })
	if err != nil {
		t.Fatalf("LoanEdit(): %v", err)
	}
	res, err := LoanShow(path, "casa")
	if err != nil {
		t.Fatalf("LoanShow(): %v", err)
	}
	if res.Loan.Spread != nuevoSpread {
		t.Fatalf("spread = %q, want %q", res.Loan.Spread, nuevoSpread)
	}
	if res.Loan.Name != "Casa" {
		t.Fatalf("edit no debía tocar el nombre: %q", res.Loan.Name)
	}

	err = LoanEdit(path, "noexiste", func(l *loan.Loan) {})
	assertAppError(t, err, errs.KindNotFound, 4)
}

// TestLoanRemoveConConfirmacion: sin --yes nada se borra y responde
// confirmation_required/2 (decisión 3 de LOAN-01); con --yes se borra.
func TestLoanRemoveConConfirmacion(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoCompleto()})

	_, err := LoanRemove(path, "casa", false)
	e := assertAppError(t, err, errs.KindConfirmationRequired, 2)
	if !containsAll(e.Hint, "--yes") {
		t.Fatalf("el hint debe apuntar a --yes: %q", e.Hint)
	}
	// El préstamo sigue intacto tras el rechazo.
	if _, err := LoanShow(path, "casa"); err != nil {
		t.Fatalf("el préstamo debía seguir guardado: %v", err)
	}

	if _, err := LoanRemove(path, "casa", true); err != nil {
		t.Fatalf("LoanRemove() con confirmación: %v", err)
	}
	_, err = LoanShow(path, "casa")
	assertAppError(t, err, errs.KindNotFound, 4)
}

// TestLoanSyncGuardaYValida: sync guarda los tres campos del recibo y
// rechaza valores imposibles (LOAN-01.3).
func TestLoanSyncGuardaYValida(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoCompleto()})

	s := loan.Sync{Date: "2026-10-01", OutstandingPrincipal: "152341.18", CurrentRate: "3.12"}
	if err := LoanSync(path, "casa", s); err != nil {
		t.Fatalf("LoanSync(): %v", err)
	}
	res, err := LoanShow(path, "casa")
	if err != nil {
		t.Fatalf("LoanShow(): %v", err)
	}
	if res.Loan.Sync == nil || res.Loan.Sync.Date != s.Date ||
		res.Loan.Sync.OutstandingPrincipal != s.OutstandingPrincipal ||
		res.Loan.Sync.CurrentRate != s.CurrentRate {
		t.Fatalf("sync = %+v, want %+v", res.Loan.Sync, s)
	}

	malo := loan.Sync{Date: "ayer", OutstandingPrincipal: "152341.18", CurrentRate: "3.12"}
	if err := LoanSync(path, "casa", malo); err == nil {
		t.Fatal("LoanSync() con fecha inválida: error = nil, want usage")
	}
	// El sync anterior sigue ahí tras el rechazo.
	res, err = LoanShow(path, "casa")
	if err != nil || res.Loan.Sync.Date != s.Date {
		t.Fatalf("el sync válido debía seguir guardado: %+v err=%v", res.Loan.Sync, err)
	}
}

// TestLoanPointOfPuntoActual: el punto de cálculo parte del recibo cuando
// hay sync (decisión 2 de LOAN-01) y del principio sin sync; un préstamo
// incompleto responde loan_incomplete/2 nombrando el campo.
func TestLoanPointOfPuntoActual(t *testing.T) {
	conSync := prestamoCompleto()
	conSync.Sync = &loan.Sync{Date: "2026-10-01", OutstandingPrincipal: "152341.18", CurrentRate: "3.12"}
	path := configCon(t, map[string]loan.Loan{"casa": conSync})

	point, err := LoanPointOf(path, "")
	if err != nil {
		t.Fatalf("LoanPointOf(): %v", err)
	}
	if point.ID != "casa" {
		t.Fatalf("id = %q, want casa", point.ID)
	}
	if !point.Principal.Decimal().Equal(decimal.RequireFromString("152341.18")) {
		t.Fatalf("principal = %s, want el capital del recibo", point.Principal)
	}
	if !point.AnnualRatePct.Decimal().Equal(decimal.RequireFromString("3.12")) {
		t.Fatalf("rate = %s, want el tipo del recibo", point.AnnualRatePct)
	}
	// Meses entre 2021-06 y 2026-10: 64; restan 360-64 = 296.
	if point.RemainingMonths != 296 {
		t.Fatalf("remaining = %d, want 296", point.RemainingMonths)
	}
	if !point.FeePct.Decimal().IsZero() {
		t.Fatalf("fee = %s, want 0", point.FeePct)
	}

	sinSync := configCon(t, map[string]loan.Loan{"casa": prestamoCompleto()})
	point, err = LoanPointOf(sinSync, "casa")
	if err != nil {
		t.Fatalf("LoanPointOf() sin sync: %v", err)
	}
	if !point.Principal.Decimal().Equal(decimal.RequireFromString("180000.00")) || point.RemainingMonths != 360 {
		t.Fatalf("punto sin sync = %s/%d, want 180000.00/360", point.Principal, point.RemainingMonths)
	}

	incompleto := prestamoCompleto()
	incompleto.FloorRate = nil
	pathInc := configCon(t, map[string]loan.Loan{"casa": incompleto})
	_, err = LoanPointOf(pathInc, "casa")
	e := assertAppError(t, err, errs.KindLoanIncomplete, 2)
	if !containsAll(e.Message, "floor_rate") || !containsAll(e.Hint, "floor_rate") {
		t.Fatalf("loan_incomplete debe nombrar el campo: %q / %q", e.Message, e.Hint)
	}
}

// TestRevisionSinPrestamosPreservaElStub: `revision` y `schedule` sin ningún
// préstamo guardado mantienen la respuesta honesta del stub de CMD-01
// (loan_incomplete/2 con el hint de `loan add`).
func TestRevisionSinPrestamosPreservaElStub(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	_, err := Revision(t.Context(), &Runtime{}, RevisionParams{ConfigPath: path})
	e := assertAppError(t, err, errs.KindLoanIncomplete, 2)
	if !containsAll(e.Hint, "loan add") {
		t.Fatalf("el hint debe apuntar a \"loan add\": %q", e.Hint)
	}
	// El hint ya no cita una tarea: apunta al comando que arregla el estado.
	if containsAll(e.Hint, "LOAN-01") {
		t.Fatalf("el hint no debe citar una tarea interna: %q", e.Hint)
	}
	_, err = Schedule(ScheduleParams{ConfigPath: path})
	assertAppError(t, err, errs.KindLoanIncomplete, 2)
}

// TestLoanRemoveIdOpcional: con un único préstamo guardado, omitir el id
// borra ése (LOAN-01.4), siempre con --yes: sin ella nada se toca. Con
// varios préstamos y sin id, usage/2 listando los ids.
func TestLoanRemoveIdOpcional(t *testing.T) {
	solo := configCon(t, map[string]loan.Loan{"casa": prestamoCompleto()})

	// Sin --yes: confirmation_required/2 y el préstamo sigue intacto.
	_, err := LoanRemove(solo, "", false)
	assertAppError(t, err, errs.KindConfirmationRequired, 2)
	if _, err := LoanShow(solo, "casa"); err != nil {
		t.Fatalf("el préstamo debía seguir guardado: %v", err)
	}

	// Con --yes y un único préstamo: se borra ése.
	id, err := LoanRemove(solo, "", true)
	if err != nil || id != "casa" {
		t.Fatalf("LoanRemove() sin id = %q, %v; want casa, nil", id, err)
	}
	_, err = LoanShow(solo, "casa")
	assertAppError(t, err, errs.KindNotFound, 4)

	// Con varios préstamos y sin id: usage/2 listando los ids.
	varios := configCon(t, map[string]loan.Loan{
		"casa": prestamoCompleto(),
		"piso": prestamoCompleto(),
	})
	_, err = LoanRemove(varios, "", true)
	e := assertAppError(t, err, errs.KindUsage, 2)
	if !containsAll(e.Message, "casa", "piso") {
		t.Fatalf("el error debe listar los ids: %q", e.Message)
	}
}

// assertAppError comprueba el sobre del contrato (kind y código).
func assertAppError(t *testing.T, err error, kind errs.Kind, code int) *errs.Error {
	t.Helper()
	if err == nil {
		t.Fatal("se esperaba un error, no hubo")
	}
	var e *errs.Error
	if !errors.As(err, &e) {
		t.Fatalf("el error debe llevar kind del contrato, got %v (%T)", err, err)
	}
	if e.Kind != kind {
		t.Fatalf("kind = %q, want %q (mensaje: %s)", e.Kind, kind, e.Message)
	}
	if got := errs.ExitCode(e.Kind); got != code {
		t.Fatalf("código = %d, want %d", got, code)
	}
	return e
}

// containsAll dice si s contiene todas las subcadenas.
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
