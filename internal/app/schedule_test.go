package app

import (
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/loan"
)

// ----------------------------------------------------------------------------
// `schedule` desde un préstamo guardado (LOAN-01.6). El préstamo de partida
// es el caso 1 de testdata/finance (150000.00 a 3.10 % a 300 meses): sus
// cifras son de referencia independiente y no se tocan.
// ----------------------------------------------------------------------------

// prestamoCaso01 construye el préstamo del caso 1 de testdata/finance como
// préstamo fijo guardado.
func prestamoCaso01() loan.Loan {
	tramo := 300
	sinSuelo := ""
	return loan.Loan{
		Name:                  "Caso 1",
		Type:                  loan.TypeFija,
		StartDate:             "2026-01-01",
		OriginalPrincipal:     "150000.00",
		TermMonths:            300,
		PaymentDay:            1,
		InitialRate:           "3.10",
		InitialPeriodMonths:   &tramo,
		Index:                 "euribor-1y",
		Spread:                "0.00",
		RevisionEveryMonths:   12,
		ReferenceOffsetMonths: func() *int { o := 2; return &o }(),
		FloorRate:             &sinSuelo,
		RateRounding:          "none",
		InterestConvention:    loan.InterestConventionMonthlyNominal,
		PaymentRounding:       loan.PaymentRoundingHalfUpCents,
		EarlyRepaymentFeePct:  "0",
	}
}

// TestScheduleDesdeCaso01: el cuadro del préstamo guardado coincide con el
// caso de referencia 1 al céntimo (primera fila, última fila y totales).
func TestScheduleDesdeCaso01(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"caso": prestamoCaso01()})

	res, err := Schedule(ScheduleParams{ConfigPath: path, LoanID: "caso"})
	if err != nil {
		t.Fatalf("Schedule(): %v", err)
	}
	if res.Loan != "caso" || res.Months != 300 || len(res.Rows) != 300 {
		t.Fatalf("cabecera = loan %s months %d rows %d, want caso/300/300", res.Loan, res.Months, len(res.Rows))
	}
	if res.AnnualRatePct.String() != "3.10" {
		t.Fatalf("annual_rate_pct = %s, want 3.10", res.AnnualRatePct)
	}
	if res.StartDate != "2026-01-01" {
		t.Fatalf("start_date = %q, want 2026-01-01", res.StartDate)
	}

	first := res.Rows[0]
	if first.Month != 1 || first.Date != "2026-01-01" ||
		first.Payment.String() != "719.14" || first.Interest.String() != "387.50" ||
		first.Principal.String() != "331.64" || first.Balance.String() != "149668.36" {
		t.Fatalf("primera fila = %+v, want la del caso 1", first)
	}
	last := res.Rows[len(res.Rows)-1]
	if last.Month != 300 || last.Payment.String() != "720.70" ||
		last.Interest.String() != "1.86" || last.Principal.String() != "718.84" ||
		last.Balance.String() != "0.00" {
		t.Fatalf("última fila = %+v, want la del caso 1", last)
	}
	if res.TotalInterest.String() != "65743.56" || res.TotalPaid.String() != "215743.56" {
		t.Fatalf("totales = %s/%s, want 65743.56/215743.56", res.TotalInterest, res.TotalPaid)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want vacío (préstamo fijo)", res.Warnings)
	}
	if res.Attribution.NotEndorsed != true ||
		res.Attribution.Text != "Elaboración propia con datos extraídos del sitio web del Banco de España (www.bde.es)" {
		t.Fatalf("attribution = %+v", res.Attribution)
	}
}

// TestScheduleRecorteFromTo: --from/--to recortan las filas mostradas por
// mes, ambos incluidos; los totales siguen siendo los del cuadro completo.
func TestScheduleRecorteFromTo(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"caso": prestamoCaso01()})

	res, err := Schedule(ScheduleParams{ConfigPath: path, LoanID: "caso", From: "2028-01", To: "2028-03"})
	if err != nil {
		t.Fatalf("Schedule(): %v", err)
	}
	if len(res.Rows) != 3 {
		t.Fatalf("filas = %d, want 3", len(res.Rows))
	}
	if res.Rows[0].Date != "2028-01-01" || res.Rows[2].Date != "2028-03-01" {
		t.Fatalf("recorte = %q..%q, want 2028-01-01..2028-03-01", res.Rows[0].Date, res.Rows[2].Date)
	}
	// El mes de la primera fila del recorte: 24 cuotas después de la primera.
	if res.Rows[0].Month != 25 {
		t.Fatalf("month = %d, want 25", res.Rows[0].Month)
	}
	if res.TotalInterest.String() != "65743.56" {
		t.Fatalf("los totales siguen siendo los del cuadro completo: %s", res.TotalInterest)
	}

	// Formato YYYY-MM-DD y validación del orden.
	if _, err := Schedule(ScheduleParams{ConfigPath: path, LoanID: "caso", From: "2028-01-15"}); err != nil {
		t.Fatalf("Schedule() con --date completa: %v", err)
	}
	_, err = Schedule(ScheduleParams{ConfigPath: path, LoanID: "caso", From: "2028-03", To: "2028-01"})
	if err == nil {
		t.Fatal("--to anterior a --from: error = nil, want usage")
	}
}

// TestScheduleVariableSinSyncAvisa: en un préstamo variable sin recibo el
// cuadro sólo describe el tramo inicial y la salida lo avisa (P-04) en lugar
// de presentarse como definitivo.
func TestScheduleVariableSinSyncAvisa(t *testing.T) {
	path := configCon(t, map[string]loan.Loan{"casa": prestamoCompleto()})

	res, err := Schedule(ScheduleParams{ConfigPath: path, LoanID: "casa"})
	if err != nil {
		t.Fatalf("Schedule(): %v", err)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings = %v, want el aviso del tipo inicial", res.Warnings)
	}
	if res.Months != 360 || len(res.Rows) != 360 {
		t.Fatalf("cuadro = %d/%d filas, want 360/360", res.Months, len(res.Rows))
	}
}

// TestScheduleDesdeSync: con recibo, el cuadro parte del capital y tipo del
// recibo y dura los meses restantes (decisión 2 de LOAN-01), sin aviso.
func TestScheduleDesdeSync(t *testing.T) {
	l := prestamoCompleto()
	l.Sync = &loan.Sync{Date: "2026-10-01", OutstandingPrincipal: "152341.18", CurrentRate: "3.12"}
	path := configCon(t, map[string]loan.Loan{"casa": l})

	res, err := Schedule(ScheduleParams{ConfigPath: path, LoanID: "casa"})
	if err != nil {
		t.Fatalf("Schedule(): %v", err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %v, want vacío (el recibo manda)", res.Warnings)
	}
	if res.Months != 296 || len(res.Rows) != 296 {
		t.Fatalf("cuadro = %d filas, want 296 (360 - 64 meses desde el recibo)", res.Months)
	}
	if res.StartDate != "2026-10-01" {
		t.Fatalf("start_date = %q, want la fecha del recibo", res.StartDate)
	}
	if res.Rows[0].Date != "2026-11-01" {
		t.Fatalf("primera cuota = %s, want 2026-11-01 (el mes del recibo ya se pagó)", res.Rows[0].Date)
	}
}

// TestScheduleIncompletoYPrestamos: schedule con un préstamo incompleto da
// loan_incomplete/2 y con varios préstamos y sin id, usage/2 listando ids.
func TestScheduleIncompletoYPrestamos(t *testing.T) {
	incompleto := prestamoCaso01()
	incompleto.EarlyRepaymentFeePct = ""
	path := configCon(t, map[string]loan.Loan{"caso": incompleto})
	_, err := Schedule(ScheduleParams{ConfigPath: path, LoanID: "caso"})
	e := assertAppError(t, err, errs.KindLoanIncomplete, 2)
	if !containsAll(e.Message, "early_repayment_fee_pct") {
		t.Fatalf("el mensaje debe nombrar el campo: %q", e.Message)
	}

	varios := configCon(t, map[string]loan.Loan{
		"uno": prestamoCaso01(),
		"dos": prestamoCaso01(),
	})
	_, err = Schedule(ScheduleParams{ConfigPath: varios})
	assertAppError(t, err, errs.KindUsage, 2)
}
