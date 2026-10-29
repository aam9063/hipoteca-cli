package app

import (
	"fmt"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/finance"
	"github.com/aam9063/hipoteca-cli/internal/loan"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// ScheduleParams son los parámetros de `schedule` (LOAN-01.6). ConfigPath es
// la ruta de la config local; LoanID es opcional (con un único préstamo
// guardado se omite); From y To recortan el cuadro por mes ("YYYY-MM" o
// "YYYY-MM-DD", ambos incluidos).
type ScheduleParams struct {
	ConfigPath string
	LoanID     string
	From       string
	To         string
}

// ScheduleRow es una mensualidad del cuadro, con su fecha de pago.
type ScheduleRow struct {
	Month     int           `json:"month"`
	Date      string        `json:"date"`
	Payment   model.Decimal `json:"payment"`
	Interest  model.Decimal `json:"interest"`
	Principal model.Decimal `json:"principal_repaid"`
	Balance   model.Decimal `json:"outstanding_after"`
}

// ScheduleResult es la salida de `schedule`: el tipo de partida, el cuadro
// (completo o recortado con --from/--to), los totales del cuadro completo y
// la atribución de datos procesados (P-05).
type ScheduleResult struct {
	SchemaVersion int               `json:"schema_version"`
	Loan          string            `json:"loan"`
	AnnualRatePct model.Decimal     `json:"annual_rate_pct"`
	StartDate     string            `json:"start_date"`
	Months        int               `json:"months"`
	Rows          []ScheduleRow     `json:"rows"`
	TotalInterest model.Decimal     `json:"total_interest"`
	TotalPaid     model.Decimal     `json:"total_paid"`
	Warnings      []string          `json:"warnings"`
	Attribution   model.Attribution `json:"attribution"`
}

// Schedule construye el cuadro de amortización de un préstamo guardado con
// finance.Amortization (LOAN-01.6):
//
//   - con sync, el cuadro parte del recibo: capital pendiente, tipo vigente
//     y los meses restantes hasta el fin del plazo (decisión 2 de LOAN-01);
//   - sin sync, parte del principio con el capital original y el tipo
//     inicial. En un préstamo variable o mixto eso sólo describe el tramo
//     inicial: las revisiones posteriores necesitan índices históricos que
//     no están guardados, y la salida lo avisa en warnings en lugar de
//     presentar el cuadro como definitivo (P-04).
//
// Los totales son siempre los del cuadro completo; --from/--to sólo recorta
// las filas mostradas.
func Schedule(p ScheduleParams) (ScheduleResult, error) {
	cfg, err := loadConfigAt(p.ConfigPath)
	if err != nil {
		return ScheduleResult{}, err
	}
	if len(cfg.Loans) == 0 {
		return ScheduleResult{}, ErrLoanIncomplete("schedule")
	}
	id, err := resolveLoanID(cfg.Loans, p.LoanID)
	if err != nil {
		return ScheduleResult{}, err
	}
	l := cfg.Loans[id]
	if err := l.Validate(); err != nil {
		return ScheduleResult{}, loanError(id, err)
	}

	balance, rate, remaining, err := currentBalance(l)
	if err != nil {
		return ScheduleResult{}, err
	}
	schedule, err := finance.Amortization(finance.Loan{
		Principal:  balance,
		AnnualRate: rate,
		Months:     remaining,
	})
	if err != nil {
		return ScheduleResult{}, calcUsageError(err)
	}

	var baseDate time.Time
	var warnings []string
	if l.Sync != nil {
		// El recibo ya cobró la cuota de su mes: la primera cuota del cuadro
		// es la del mes siguiente.
		syncDate, _ := time.Parse("2006-01-02", l.Sync.Date)
		baseDate = addMonths(syncDate, 1, l.PaymentDay)
	} else {
		baseDate, _ = time.Parse("2006-01-02", l.StartDate)
		if l.Type != loan.TypeFija {
			warnings = append(warnings,
				"el cuadro usa el tipo inicial del préstamo: en un préstamo "+l.Type+
					" las revisiones posteriores al tramo inicial no están incluidas (sincroniza con tu recibo para partir del capital y tipo reales)")
		}
	}

	fromMonth, toMonth, err := parseFromTo(p.From, p.To)
	if err != nil {
		return ScheduleResult{}, err
	}

	rows := make([]ScheduleRow, 0, len(schedule.Rows))
	for i, row := range schedule.Rows {
		date := addMonths(baseDate, i, l.PaymentDay)
		if fromMonth != nil && monthKey(date) < *fromMonth {
			continue
		}
		if toMonth != nil && monthKey(date) > *toMonth {
			continue
		}
		rows = append(rows, ScheduleRow{
			Month:     row.Month,
			Date:      date.Format("2006-01-02"),
			Payment:   model.NewMoney(row.Payment),
			Interest:  model.NewMoney(row.Interest),
			Principal: model.NewMoney(row.Principal),
			Balance:   model.NewMoney(row.Balance),
		})
	}
	attr, err := model.ProcessedAttribution(time.Now().UTC().Format("2006-01-02"))
	if err != nil {
		// La fecha sale de time.Now().UTC().Format: nunca ocurre.
		return ScheduleResult{}, errs.New(errs.KindInternal, err.Error(), "")
	}
	startDate := baseDate.Format("2006-01-02")
	if l.Sync != nil {
		// La fecha de partida del cuadro con recibo es la del recibo; su
		// primera cuota es la del mes siguiente (ver arriba).
		startDate = l.Sync.Date
	}
	return ScheduleResult{
		SchemaVersion: model.SchemaVersion,
		Loan:          id,
		AnnualRatePct: model.FromDecimal(rate),
		StartDate:     startDate,
		Months:        remaining,
		Rows:          rows,
		TotalInterest: model.NewMoney(schedule.TotalInterest),
		TotalPaid:     model.NewMoney(schedule.TotalPaid),
		Warnings:      warnings,
		Attribution:   attr,
	}, nil
}

// parseFromTo traduce --from/--to a claves de mes (año*12+mes) para comparar
// con la fecha de cada cuota. Ambos formatos son válidos: YYYY-MM y
// YYYY-MM-DD (el día no entra en el recorte).
func parseFromTo(from, to string) (*int, *int, error) {
	parse := func(value, flag string) (*int, error) {
		if value == "" {
			return nil, nil
		}
		var t time.Time
		var err error
		if t, err = time.Parse("2006-01", value); err != nil {
			if t, err = time.Parse("2006-01-02", value); err != nil {
				return nil, errs.New(errs.KindUsage,
					fmt.Sprintf("la fecha %q de --%s no es correcta", value, flag),
					"usa un mes YYYY-MM o una fecha YYYY-MM-DD, p. ej. --from 2027-01")
			}
		}
		key := t.Year()*12 + int(t.Month())
		return &key, nil
	}
	fromKey, err := parse(from, "from")
	if err != nil {
		return nil, nil, err
	}
	toKey, err := parse(to, "to")
	if err != nil {
		return nil, nil, err
	}
	if fromKey != nil && toKey != nil && *toKey < *fromKey {
		return nil, nil, errs.New(errs.KindUsage,
			fmt.Sprintf("--to %s es anterior a --from %s", to, from), "")
	}
	return fromKey, toKey, nil
}

// monthKey es la clave de mes de una fecha (año*12+mes).
func monthKey(t time.Time) int {
	return t.Year()*12 + int(t.Month())
}
