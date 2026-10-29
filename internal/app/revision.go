package app

import (
	"context"
	"fmt"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/finance"
	"github.com/aam9063/hipoteca-cli/internal/loan"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/shopspring/decimal"
)

// RevisionParams son los parámetros de `revision` (LOAN-01.5). ConfigPath es
// la ruta de la config local (el CLI la resuelve con Deps.ConfigPath);
// LoanID es opcional (con un único préstamo guardado se omite); Date es la
// fecha de revisión ("YYYY-MM" o "YYYY-MM-DD") o vacío para la próxima;
// Assume es el valor de índice supuesto (--assume) o nil para usar el
// publicado; Now es el reloj inyectado.
type RevisionParams struct {
	ConfigPath string
	LoanID     string
	Date       string
	Assume     *model.Decimal
	Now        time.Time
}

// RevisionIndexBlock es el bloque `index` de la salida (arranque §5.2).
type RevisionIndexBlock struct {
	ID             string        `json:"id"`
	ReferenceMonth string        `json:"reference_month"`
	Value          model.Decimal `json:"value"`
	Status         string        `json:"status"`
	Series         string        `json:"series"`
	PublishedAt    string        `json:"published_at,omitempty"`
}

// RevisionRateBlock es la cadena del tipo, cada paso expuesto (P-04).
type RevisionRateBlock struct {
	Previous      model.Decimal `json:"previous"`
	Spread        model.Decimal `json:"spread"`
	Raw           model.Decimal `json:"raw"`
	AfterFloor    model.Decimal `json:"after_floor"`
	AfterRounding model.Decimal `json:"after_rounding"`
	New           model.Decimal `json:"new"`
}

// RevisionInputsBlock son las entradas del recálculo de la cuota (P-04).
type RevisionInputsBlock struct {
	OutstandingPrincipal model.Decimal `json:"outstanding_principal"`
	RemainingMonths      int           `json:"remaining_months"`
	OutstandingSource    string        `json:"outstanding_source"`
	InterestConvention   string        `json:"interest_convention"`
}

// RevisionPaymentBlock es la cuota antes y después de la revisión.
type RevisionPaymentBlock struct {
	Previous        model.Decimal `json:"previous"`
	New             model.Decimal `json:"new"`
	DifferenceMonth model.Decimal `json:"difference_month"`
	DifferenceYear  model.Decimal `json:"difference_year"`
}

// RevisionResult es la salida de `revision` con la forma del ejemplo del
// arranque (§5.2): índice y mes de referencia, cadena del tipo, entradas,
// cuota antes/después, avisos y atribución.
type RevisionResult struct {
	SchemaVersion int                  `json:"schema_version"`
	Loan          string               `json:"loan"`
	RevisionDate  string               `json:"revision_date"`
	Index         RevisionIndexBlock   `json:"index"`
	Rate          RevisionRateBlock    `json:"rate"`
	Inputs        RevisionInputsBlock  `json:"inputs"`
	Payment       RevisionPaymentBlock `json:"payment"`
	Warnings      []string             `json:"warnings"`
	Attribution   model.Attribution    `json:"attribution"`
}

// Revision calcula la revisión de la cuota de un préstamo guardado:
//
//  1. mes de referencia = mes de revisión − reference_offset_months
//     (finance.ReferenceMonth);
//  2. valor del índice de ese mes en la serie del Banco de España, o el
//     supuesto del usuario con --assume (P-06: marcado assumed y avisado);
//  3. TIN nuevo con la cadena NewRate: índice + diferencial → suelo →
//     redondeo de escritura (convención 1.5 de docs/calculos.md);
//  4. cuota nueva con el capital pendiente y los meses restantes en la fecha
//     de revisión; el capital sale del recibo (sync) cuando existe
//     (decisión 2 de LOAN-01) y del cuadro propio del préstamo en el tramo
//     inicial cuando no.
//
// Un mes de referencia sin valor publicado es index_not_published/4 con el
// hint de --assume: nunca se inventa un índice (P-06).
func Revision(ctx context.Context, rt *Runtime, p RevisionParams) (RevisionResult, error) {
	cfg, err := loadConfigAt(p.ConfigPath)
	if err != nil {
		return RevisionResult{}, err
	}
	if len(cfg.Loans) == 0 {
		return RevisionResult{}, ErrLoanIncomplete("revision")
	}
	id, err := resolveLoanID(cfg.Loans, p.LoanID)
	if err != nil {
		return RevisionResult{}, err
	}
	l := cfg.Loans[id]
	if err := l.Validate(); err != nil {
		return RevisionResult{}, loanError(id, err)
	}

	start, err := time.Parse("2006-01-02", l.StartDate)
	if err != nil {
		return RevisionResult{}, errs.New(errs.KindUsage,
			fmt.Sprintf("la fecha de inicio %q no es una fecha YYYY-MM-DD", l.StartDate), "")
	}
	revisionDate, elapsed, err := resolveRevisionDate(l, p.Date, p.Now)
	if err != nil {
		return RevisionResult{}, err
	}
	remaining := l.TermMonths - elapsed
	if remaining < 1 {
		return RevisionResult{}, errs.New(errs.KindUsage,
			fmt.Sprintf("la revisión del %s queda fuera del plazo del préstamo", revisionDate.Format("2006-01")), "")
	}

	// Capital pendiente y cuota previa en la fecha de revisión: del recibo
	// (sync) cuando cubre la fecha, del cuadro propio en el tramo inicial
	// cuando no — y en cualquier fecha para un préstamo fijo, cuyo cuadro es
	// exacto. Una revisión posterior al tramo inicial de un variable sin
	// sync no se calcula: los índices de las revisiones intermedias no
	// están guardados y suponerlos inventaría un capital (P-06).
	balance, previousPayment, previousRate, source, err := outstandingAt(l, start, elapsed)
	if err != nil {
		return RevisionResult{}, err
	}

	// Mes de referencia y valor del índice.
	offset := *l.ReferenceOffsetMonths
	refMonth := finance.ReferenceMonth(revisionDate, offset)
	// §5.2: sin avisos, warnings es una lista vacía, no null.
	warnings := []string{}
	var indexValue decimal.Decimal
	var updatedAt string
	indexBlock := RevisionIndexBlock{
		ID:             l.Index,
		ReferenceMonth: refMonth.Format("2006-01"),
		Status:         "published",
	}
	if idx, err := rt.Catalog.Lookup(l.Index); err == nil {
		indexBlock.Series = idx.Code
	} else {
		return RevisionResult{}, err
	}
	if p.Assume != nil {
		indexValue = p.Assume.Decimal()
		indexBlock.Status = "assumed"
		indexBlock.Value = *p.Assume
		warnings = append(warnings,
			"el valor del índice es un supuesto tuyo con --assume: no es un dato publicado por el Banco de España (P-06)")
		// Sin dato del Banco no hay fecha de actualización que citar: la
		// atribución data la elaboración del escenario.
		updatedAt = p.Now.UTC().Format("2006-01-02")
	} else {
		series, err := rt.Source.History(ctx, l.Index, bde.RangoMAX)
		if err != nil {
			return RevisionResult{}, err
		}
		obs, ok := observationAt(series, refMonth)
		if !ok {
			return RevisionResult{}, errs.New(errs.KindIndexNotPublished,
				fmt.Sprintf("el índice %s de %s aún no está publicado", l.Index, refMonth.Format("2006-01")),
				fmt.Sprintf("usa --assume <valor> para un escenario, p. ej. \"hipoteca revision %s --assume 2.35\"", id))
		}
		indexValue = obs.Value.Decimal()
		indexBlock.Value = obs.Value
		indexBlock.PublishedAt = obs.Month.Format("2006-01-02")
		updatedAt = series.End.Format("2006-01-02")
	}

	// Cadena del tipo y cuota nueva con el capital pendiente y los meses
	// restantes (convención 1.5 y 1.4 de docs/calculos.md).
	rate, err := rateOf(l, indexValue)
	if err != nil {
		return RevisionResult{}, loanError(id, err)
	}
	newSchedule, err := finance.Amortization(finance.Loan{
		Principal:  balance,
		AnnualRate: rate.Final,
		Months:     remaining,
	})
	if err != nil {
		return RevisionResult{}, calcUsageError(err)
	}
	newPayment := newSchedule.Rows[0].Payment
	differenceMonth := newPayment.Sub(previousPayment)
	differenceYear := differenceMonth.Mul(decimal.NewFromInt(12))

	spread, err := model.NewDecimal(l.Spread)
	if err != nil {
		return RevisionResult{}, loanError(id, err)
	}
	attr, err := model.ProcessedAttribution(updatedAt)
	if err != nil {
		return RevisionResult{}, errs.New(errs.KindInternal, err.Error(), "")
	}
	return RevisionResult{
		SchemaVersion: model.SchemaVersion,
		Loan:          id,
		RevisionDate:  revisionDate.Format("2006-01-02"),
		Index:         indexBlock,
		Rate: RevisionRateBlock{
			Previous:      model.FromDecimal(previousRate),
			Spread:        spread,
			Raw:           model.FromDecimal(rate.Bruto),
			AfterFloor:    model.FromDecimal(rate.ConSuelo),
			AfterRounding: model.FromDecimal(rate.Final),
			New:           model.FromDecimal(rate.Final),
		},
		Inputs: RevisionInputsBlock{
			OutstandingPrincipal: model.NewMoney(balance),
			RemainingMonths:      remaining,
			OutstandingSource:    source,
			InterestConvention:   l.InterestConvention,
		},
		Payment: RevisionPaymentBlock{
			Previous:        model.NewMoney(previousPayment),
			New:             model.NewMoney(newPayment),
			DifferenceMonth: model.NewMoney(differenceMonth),
			DifferenceYear:  model.NewMoney(differenceYear),
		},
		Warnings:    warnings,
		Attribution: attr,
	}, nil
}

// resolveRevisionDate resuelve la fecha de revisión: la pedida con --date
// (YYYY-MM o YYYY-MM-DD, con el día de pago cuando el mes va sin día) o la
// próxima revisión posterior al mes en curso. Devuelve también los meses
// transcurridos desde el inicio del préstamo hasta esa fecha.
func resolveRevisionDate(l loan.Loan, dateFlag string, now time.Time) (time.Time, int, error) {
	start, _ := time.Parse("2006-01-02", l.StartDate)
	if dateFlag != "" {
		var revision time.Time
		if t, err := time.Parse("2006-01-02", dateFlag); err == nil {
			revision = t
		} else if t, err := time.Parse("2006-01", dateFlag); err == nil {
			revision = time.Date(t.Year(), t.Month(), l.PaymentDay, 0, 0, 0, 0, time.UTC)
		} else {
			return time.Time{}, 0, errs.New(errs.KindUsage,
				fmt.Sprintf("la fecha %q no es correcta", dateFlag),
				"usa un mes YYYY-MM o una fecha YYYY-MM-DD, p. ej. --date 2027-01")
		}
		elapsed := monthsBetween(start, revision)
		if elapsed < 0 {
			return time.Time{}, 0, errs.New(errs.KindUsage,
				fmt.Sprintf("la fecha %q es anterior al inicio del préstamo (%s)", dateFlag, l.StartDate), "")
		}
		return revision, elapsed, nil
	}

	// Próxima revisión: la primera cuya mensualidad sea la del mes en curso
	// o una posterior. Sin revisiones dentro del plazo (tramo inicial que
	// agota el préstamo) no hay próxima revisión que calcular.
	for _, e := range revisionMonthFromStart(l) {
		candidate := addMonths(start, e, l.PaymentDay)
		if (candidate.Year()*12 + int(candidate.Month())) >= (now.Year()*12 + int(now.Month())) {
			return candidate, e, nil
		}
	}
	return time.Time{}, 0, errs.New(errs.KindUsage,
		"el préstamo no tiene ninguna revisión posterior al mes en curso: el plazo termina antes",
		"pide una fecha concreta con --date YYYY-MM")
}

// outstandingAt devuelve el capital pendiente, la cuota vigente y el tipo
// vigente para la revisión de la mensualidad elapsed (contada desde el
// inicio del préstamo, 0 = mes de la primera cuota). La cuota nueva se
// calcula sobre el capital pendiente tras la cuota del mes anterior
// (elapsed−1), que es la última pagada al tipo viejo:
//
//   - con sync que cubra la fecha (mes del recibo < mes de revisión), el
//     capital de partida es el del recibo — ya es el pendiente tras la cuota
//     del mes del recibo (decisión 2 de LOAN-01: el recibo manda sobre el
//     cuadro)— y las cuotas intermedias vienen del cuadro que parte de él
//     (outstanding_source "sync+schedule");
//   - sin sync (o con un recibo posterior a la fecha pedida), capital y
//     cuota vienen del cuadro propio del préstamo con su tipo inicial,
//     válido dentro del tramo inicial (outstanding_source "schedule");
//   - un préstamo FIJO sin sync se calcula con su cuadro propio en
//     cualquier fecha del plazo (advisory de LOAN-01): su tipo no cambia,
//     así que el cuadro es exacto y no hay ningún índice intermedio sin
//     guardar que inventar.
//
// Entre el recibo y la revisión no puede haber otra revisión: el índice de
// esa revisión intermedia no está guardado y suponerlo inventaría el
// capital (P-06). Ese mismo motivo es el que cierra el camino del cuadro
// propio a un VARIABLE (o mixto) más allá del tramo inicial: ahí las
// revisiones intermedias sí habrían cambiado el tipo.
func outstandingAt(l loan.Loan, start time.Time, elapsed int) (balance, previousPayment, previousRate decimal.Decimal, source string, err error) {
	usageRefuse := func() error {
		return errs.New(errs.KindUsage,
			"el capital pendiente en esa fecha necesita una revisión intermedia cuyo índice no está guardado",
			"sincroniza con tu último recibo: \"hipoteca loan sync --date <YYYY-MM-DD> --outstanding <capital> --rate <tipo>\"")
	}
	if l.Sync != nil && monthsBetween(start, mustParseDate(l.Sync.Date)) <= elapsed {
		syncElapsed := monthsBetween(start, mustParseDate(l.Sync.Date))
		// Revisión intermedia entre el recibo y la fecha pedida (sin contar
		// la propia fecha pedida, que es la que estamos calculando).
		for _, boundary := range revisionMonthFromStart(l) {
			if boundary > syncElapsed && boundary < elapsed {
				return decimal.Decimal{}, decimal.Decimal{}, decimal.Decimal{}, "", usageRefuse()
			}
		}
		remainingAtSync := l.TermMonths - syncElapsed
		if remainingAtSync < 1 {
			return decimal.Decimal{}, decimal.Decimal{}, decimal.Decimal{}, "",
				errs.New(errs.KindUsage, "la fecha del recibo queda fuera del plazo del préstamo", "")
		}
		syncPrincipal, err := model.NewDecimal(l.Sync.OutstandingPrincipal)
		if err != nil {
			return decimal.Decimal{}, decimal.Decimal{}, decimal.Decimal{}, "",
				errs.New(errs.KindUsage, fmt.Sprintf("el capital pendiente del recibo %q no es un decimal válido", l.Sync.OutstandingPrincipal), "")
		}
		syncRate, err := model.NewDecimal(l.Sync.CurrentRate)
		if err != nil {
			return decimal.Decimal{}, decimal.Decimal{}, decimal.Decimal{}, "",
				errs.New(errs.KindUsage, fmt.Sprintf("el tipo vigente del recibo %q no es un decimal válido", l.Sync.CurrentRate), "")
		}
		rows, err := finance.Amortization(finance.Loan{
			Principal:  syncPrincipal.Decimal(),
			AnnualRate: syncRate.Decimal(),
			Months:     remainingAtSync,
		})
		if err != nil {
			return decimal.Decimal{}, decimal.Decimal{}, decimal.Decimal{}, "", calcUsageError(err)
		}
		balance, payment := walkFromSync(rows.Rows, elapsed-syncElapsed, syncPrincipal.Decimal())
		return balance, payment, syncRate.Decimal(), "sync+schedule", nil
	}

	initialPeriod := 0
	if l.InitialPeriodMonths != nil {
		initialPeriod = *l.InitialPeriodMonths
	}
	// El rechazo por falta de cobertura sólo aplica a un préstamo cuyo tipo
	// cambia con las revisiones: en un fijo el cuadro propio es exacto en
	// cualquier fecha (advisory de LOAN-01, decisión 4 de FIN-04).
	if elapsed > initialPeriod && l.Type != loan.TypeFija {
		return decimal.Decimal{}, decimal.Decimal{}, decimal.Decimal{}, "", errs.New(errs.KindUsage,
			fmt.Sprintf("el capital pendiente en %s no se puede calcular: la revisión del mes %d no está cubierta por el tramo inicial ni por un recibo sincronizado",
				addMonths(start, elapsed, l.PaymentDay).Format("2006-01"), elapsed),
			"sincroniza con tu último recibo: \"hipoteca loan sync --date <YYYY-MM-DD> --outstanding <capital> --rate <tipo>\"")
	}
	initialRate, rateErr := model.NewDecimal(l.InitialRate)
	if rateErr != nil {
		return decimal.Decimal{}, decimal.Decimal{}, decimal.Decimal{}, "",
			errs.New(errs.KindUsage, fmt.Sprintf("el tipo inicial %q no es un decimal válido", l.InitialRate), "")
	}
	original, origErr := model.NewDecimal(l.OriginalPrincipal)
	if origErr != nil {
		return decimal.Decimal{}, decimal.Decimal{}, decimal.Decimal{}, "",
			errs.New(errs.KindUsage, fmt.Sprintf("el capital original %q no es un decimal válido", l.OriginalPrincipal), "")
	}
	rows, rowsErr := finance.Amortization(finance.Loan{
		Principal:  original.Decimal(),
		AnnualRate: initialRate.Decimal(),
		Months:     l.TermMonths,
	})
	if rowsErr != nil {
		return decimal.Decimal{}, decimal.Decimal{}, decimal.Decimal{}, "", calcUsageError(err)
	}
	// El capital para la cuota nueva es el pendiente tras la cuota del mes
	// elapsed−1; con elapsed 0 (revisión en el mes de la primera cuota) es el
	// capital original. La cuota previa es la del tipo inicial.
	balance = original.Decimal()
	if elapsed > 0 {
		balance = rows.Rows[elapsed-1].Balance
	}
	return balance, rows.Rows[0].Payment, initialRate.Decimal(), "schedule", nil
}

// walkFromSync devuelve el capital pendiente para la cuota nueva y la cuota
// vieja, contando los meses desde el recibo: delta 0 o 1 significa que no ha
// habido ninguna cuota al tipo viejo desde el recibo, así que el capital es
// el del recibo tal cual; con más meses, el pendiente tras la cuota del mes
// anterior a la revisión (la fila delta−2 del cuadro que parte del recibo).
func walkFromSync(rows []finance.Row, delta int, syncBalance decimal.Decimal) (decimal.Decimal, decimal.Decimal) {
	if delta <= 1 {
		return syncBalance, rows[0].Payment
	}
	return rows[delta-2].Balance, rows[0].Payment
}

// mustParseDate parsea una fecha YYYY-MM-DD ya validada; si no lo fuera,
// devuelve el cero y el flujo de arriba lo trata como sync ausente.
func mustParseDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

// observationAt busca el valor del mes pedido en la serie (meses
// normalizados al día 1, 00:00 UTC).
func observationAt(series bde.Series, month time.Time) (bde.Observation, bool) {
	for _, obs := range series.Observations {
		if obs.Month.Equal(month) {
			return obs, true
		}
	}
	return bde.Observation{}, false
}
