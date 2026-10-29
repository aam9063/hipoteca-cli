package app

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/finance"
	"github.com/aam9063/hipoteca-cli/internal/loan"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/shopspring/decimal"
)

// ----------------------------------------------------------------------------
// Préstamos guardados (LOAN-01): operaciones sobre la sección [loans.<id>]
// de la config local. La escritura pasa siempre por config.Load y
// config.Save (atómica, 0600, decisión 4 de LOAN-01); nunca se escribe a
// mano. Todas las operaciones reciben la ruta de la config como parámetro:
// el CLI la resuelve (config.ConfigPath, sustituible en los tests vía
// Deps.ConfigPath) y la capa de aplicación no toca el entorno.
// ----------------------------------------------------------------------------

// loadConfigAt lee la config de la ruta dada.
func loadConfigAt(path string) (config.Config, error) {
	return config.Load(path)
}

// resolveLoanID resuelve el id de préstamo para los comandos que lo aceptan
// como opcional (LOAN-01.4): con un único préstamo guardado, omitirlo es
// correcto; con varios, la ausencia de id es un error de uso que lista los
// ids disponibles; un id dado que no existe es not_found/4.
func resolveLoanID(loans map[string]loan.Loan, id string) (string, error) {
	if id != "" {
		if _, ok := loans[id]; !ok {
			return "", errs.New(errs.KindNotFound,
				fmt.Sprintf("no hay ningún préstamo guardado con el id %q", id),
				"\"hipoteca loan list\" muestra los préstamos guardados")
		}
		return id, nil
	}
	if len(loans) == 0 {
		return "", errs.New(errs.KindNotFound,
			"no hay ningún préstamo guardado",
			"guarda uno con \"hipoteca loan add\"")
	}
	if len(loans) > 1 {
		ids := make([]string, 0, len(loans))
		for k := range loans {
			ids = append(ids, k)
		}
		sort.Strings(ids)
		return "", errs.New(errs.KindUsage,
			fmt.Sprintf("hay %d préstamos guardados (%s): indica el id", len(ids), joinIDs(ids)),
			"pasa el id como argumento, p. ej. \"hipoteca loan show casa\"")
	}
	for k := range loans {
		return k, nil
	}
	return "", nil // inalcanzable: len(loans) == 1
}

// joinIDs une los ids con ", " para el mensaje de uso.
func joinIDs(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ", "
		}
		out += id
	}
	return out
}

// loanError traduce los errores de validación del modelo a errores del
// contrato: un préstamo incompleto es loan_incomplete/2 con un hint que
// nombra el campo que falta (decisión 1 de LOAN-01); los valores inválidos
// ya salen de internal/loan como usage/2 y suben tal cual.
func loanError(id string, err error) error {
	var inc *loan.IncompleteError
	if errors.As(err, &inc) {
		return errs.New(errs.KindLoanIncomplete,
			fmt.Sprintf("el préstamo %q no está completo: falta %s", id, inc.Field),
			fmt.Sprintf("completa el campo %q con \"hipoteca loan edit %s\": es un dato de tu escritura y no se supone", inc.Field, id))
	}
	return err
}

// ----------------------------------------------------------------------------
// Salidas
// ----------------------------------------------------------------------------

// LoanSummary es una fila de `loan list`.
type LoanSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Complete bool   `json:"complete"`
}

// LoanListResult es la salida de `loan list`.
type LoanListResult struct {
	SchemaVersion int           `json:"schema_version"`
	Loans         []LoanSummary `json:"loans"`
}

// LoanView es la salida de `loan show`: el préstamo guardado tal cual está
// en la config, con el id, la marca de completo y los campos que faltan.
type LoanView struct {
	loan.Loan
	ID            string   `json:"id"`
	Complete      bool     `json:"complete"`
	MissingFields []string `json:"missing_fields,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

// LoanShowResult es la salida de `loan show`.
type LoanShowResult struct {
	SchemaVersion int      `json:"schema_version"`
	Loan          LoanView `json:"loan"`
}

// ----------------------------------------------------------------------------
// Operaciones
// ----------------------------------------------------------------------------

// LoanList lista los préstamos guardados con su marca de completo. Sin
// préstamos devuelve una lista vacía (no es error: es el estado de una
// instalación nueva).
func LoanList(path string) (LoanListResult, error) {
	cfg, err := loadConfigAt(path)
	if err != nil {
		return LoanListResult{}, err
	}
	ids := make([]string, 0, len(cfg.Loans))
	for id := range cfg.Loans {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	loans := make([]LoanSummary, 0, len(ids))
	for _, id := range ids {
		l := cfg.Loans[id]
		loans = append(loans, LoanSummary{
			ID:       id,
			Name:     l.Name,
			Type:     l.Type,
			Complete: len(l.MissingFields()) == 0,
		})
	}
	return LoanListResult{SchemaVersion: model.SchemaVersion, Loans: loans}, nil
}

// LoanShow muestra un préstamo guardado. Con un préstamo incompleto no
// falla: lo muestra con missing_fields y un aviso por campo (arranque §4.1:
// show avisa si faltan); los comandos que calculan sí responden
// loan_incomplete.
func LoanShow(path, idOrEmpty string) (LoanShowResult, error) {
	cfg, err := loadConfigAt(path)
	if err != nil {
		return LoanShowResult{}, err
	}
	id, err := resolveLoanID(cfg.Loans, idOrEmpty)
	if err != nil {
		return LoanShowResult{}, err
	}
	l := cfg.Loans[id]
	missing := l.MissingFields()
	view := LoanView{
		Loan:     l,
		ID:       id,
		Complete: len(missing) == 0,
	}
	if len(missing) > 0 {
		view.MissingFields = missing
		for _, campo := range missing {
			view.Warnings = append(view.Warnings,
				fmt.Sprintf("falta %s: es un dato de tu escritura y los comandos de cálculo responderán loan_incomplete hasta que lo completes", campo))
		}
	}
	return LoanShowResult{SchemaVersion: model.SchemaVersion, Loan: view}, nil
}

// LoanAdd guarda un préstamo nuevo con el id dado. El préstamo puede llegar
// incompleto (es dato del usuario y se guarda tal cual; show lo avisa), pero
// los valores presentes deben ser válidos y el id no puede existir.
func LoanAdd(path, id string, l loan.Loan) error {
	if id == "" {
		return errs.New(errs.KindUsage,
			"falta el id del préstamo",
			"ejemplo: \"hipoteca loan add casa --name Casa --type variable ...\"")
	}
	cfg, err := loadConfigAt(path)
	if err != nil {
		return err
	}
	if _, exists := cfg.Loans[id]; exists {
		return errs.New(errs.KindUsage,
			fmt.Sprintf("ya existe un préstamo guardado con el id %q", id),
			"usa \"hipoteca loan edit\" para cambiarlo o elige otro id")
	}
	if err := l.ValidateValues(); err != nil {
		return loanError(id, err)
	}
	if cfg.Loans == nil {
		cfg.Loans = map[string]loan.Loan{}
	}
	cfg.Loans[id] = l
	return cfg.Save(path)
}

// LoanEdit aplica los cambios de la función apply sobre el préstamo guardado
// y lo vuelve a guardar. Los valores presentes tras la edición deben seguir
// siendo válidos; el préstamo puede seguir incompleto a medias.
func LoanEdit(path, id string, apply func(*loan.Loan)) error {
	cfg, err := loadConfigAt(path)
	if err != nil {
		return err
	}
	l, ok := cfg.Loans[id]
	if !ok {
		return errs.New(errs.KindNotFound,
			fmt.Sprintf("no hay ningún préstamo guardado con el id %q", id),
			"\"hipoteca loan list\" muestra los préstamos guardados")
	}
	apply(&l)
	if err := l.ValidateValues(); err != nil {
		return loanError(id, err)
	}
	cfg.Loans[id] = l
	return cfg.Save(path)
}

// LoanRemove borra el préstamo del id guardado. Sin confirmación explícita
// no toca nada y responde confirmation_required/2 (decisión 3 de LOAN-01):
// borrar el préstamo es irreversible sobre datos del usuario.
func LoanRemove(path, id string, confirm bool) error {
	cfg, err := loadConfigAt(path)
	if err != nil {
		return err
	}
	if _, ok := cfg.Loans[id]; !ok {
		return errs.New(errs.KindNotFound,
			fmt.Sprintf("no hay ningún préstamo guardado con el id %q", id),
			"\"hipoteca loan list\" muestra los préstamos guardados")
	}
	if !confirm {
		return errs.New(errs.KindConfirmationRequired,
			fmt.Sprintf("borrar el préstamo %q es irreversible: no se ha borrado nada", id),
			"vuelve a lanzar el comando con --yes para confirmar el borrado")
	}
	delete(cfg.Loans, id)
	return cfg.Save(path)
}

// LoanSync guarda la sincronización con el recibo real: fecha, capital
// pendiente y tipo vigente (decisión 2 de LOAN-01: a partir de aquí, el
// capital y el tipo de partida son los del recibo).
func LoanSync(path, idOrEmpty string, s loan.Sync) error {
	cfg, err := loadConfigAt(path)
	if err != nil {
		return err
	}
	// El id se resuelve antes de mirar los flags: con varios préstamos y
	// sin id, el error de uso lista los ids (LOAN-01.4).
	id, err := resolveLoanID(cfg.Loans, idOrEmpty)
	if err != nil {
		return err
	}
	// Los tres datos del recibo llegan juntos; uno ausente es un error de
	// uso que nombra la bandera.
	switch {
	case s.Date == "":
		return errs.New(errs.KindUsage,
			"falta --date: indica los tres datos del recibo",
			"ejemplo: hipoteca loan sync casa --date 2026-10-01 --outstanding 152341.18 --rate 3.12")
	case s.OutstandingPrincipal == "":
		return errs.New(errs.KindUsage,
			"falta --outstanding: indica los tres datos del recibo",
			"ejemplo: hipoteca loan sync casa --date 2026-10-01 --outstanding 152341.18 --rate 3.12")
	case s.CurrentRate == "":
		return errs.New(errs.KindUsage,
			"falta --rate: indica los tres datos del recibo",
			"ejemplo: hipoteca loan sync casa --date 2026-10-01 --outstanding 152341.18 --rate 3.12")
	}
	l := cfg.Loans[id]
	l.Sync = &s
	// La validación de valores incluye los campos del sync (fechas y
	// decimales); la presencia se comprueba en los propios flags del CLI.
	if err := l.ValidateValues(); err != nil {
		return loanError(id, err)
	}
	cfg.Loans[id] = l
	return cfg.Save(path)
}

// ----------------------------------------------------------------------------
// Punto actual del préstamo (lo que consumen amortize, scenario y schedule)
// ----------------------------------------------------------------------------

// LoanPoint es el punto de partida de un cálculo sobre un préstamo guardado:
// el capital pendiente, el tipo vigente y los meses restantes HOY. Con sync,
// manda el recibo (decisión 2 de LOAN-01); sin sync, el préstamo desde el
// principio con su tipo inicial. Los importes y tipos viajan como
// model.Decimal: es el tipo del contrato y el que la guarda AST de este
// paquete exige en los structs.
type LoanPoint struct {
	ID              string
	Principal       model.Decimal
	AnnualRatePct   model.Decimal
	RemainingMonths int
	FeePct          model.Decimal
	// Spread, Floor y Rounding son la cadena de revisión de la escritura,
	// para `scenario` con préstamo; Floor nil = sin suelo.
	Spread   model.Decimal
	Floor    *model.Decimal
	Rounding string
}

// LoanPointOf resuelve el punto actual del préstamo (id opcional: con uno
// solo guardado, omitirlo es válido). El préstamo debe estar completo: sin
// los datos de la escritura no hay cálculo que valga (loan_incomplete/2).
func LoanPointOf(path, idOrEmpty string) (LoanPoint, error) {
	cfg, err := loadConfigAt(path)
	if err != nil {
		return LoanPoint{}, err
	}
	id, err := resolveLoanID(cfg.Loans, idOrEmpty)
	if err != nil {
		return LoanPoint{}, err
	}
	l := cfg.Loans[id]
	if err := l.Validate(); err != nil {
		return LoanPoint{}, loanError(id, err)
	}
	balance, rate, remaining, err := currentBalance(l)
	if err != nil {
		return LoanPoint{}, err
	}
	fee, err := model.NewDecimal(l.EarlyRepaymentFeePct)
	if err != nil {
		return LoanPoint{}, loanError(id, err)
	}
	spread, err := model.NewDecimal(l.Spread)
	if err != nil {
		return LoanPoint{}, loanError(id, err)
	}
	point := LoanPoint{
		ID:              id,
		Principal:       model.FromDecimal(balance),
		AnnualRatePct:   model.FromDecimal(rate),
		RemainingMonths: remaining,
		FeePct:          model.FromDecimal(fee.Decimal()),
		Spread:          spread,
		Rounding:        l.RateRounding,
	}
	if l.FloorRate != nil && *l.FloorRate != "" {
		floor, err := model.NewDecimal(*l.FloorRate)
		if err != nil {
			return LoanPoint{}, loanError(id, err)
		}
		point.Floor = &floor
	}
	return point, nil
}

// currentBalance devuelve el capital pendiente y el tipo de partida del
// préstamo hoy, con los meses restantes hasta el fin del plazo:
//
//   - con sync, los del recibo, y los meses restantes contados desde la
//     fecha del recibo (decisión 2 de LOAN-01);
//   - sin sync, el capital original y el tipo inicial, contados desde el
//     principio del plazo.
func currentBalance(l loan.Loan) (decimal.Decimal, decimal.Decimal, int, error) {
	var principalValue, rateValue string
	var elapsed int
	if l.Sync != nil {
		syncDate, err := time.Parse("2006-01-02", l.Sync.Date)
		if err != nil {
			return decimal.Decimal{}, decimal.Decimal{}, 0, errs.New(errs.KindUsage,
				fmt.Sprintf("la fecha del recibo %q no es una fecha YYYY-MM-DD", l.Sync.Date), "")
		}
		startDate, _ := time.Parse("2006-01-02", l.StartDate)
		elapsed = monthsBetween(startDate, syncDate)
		principalValue = l.Sync.OutstandingPrincipal
		rateValue = l.Sync.CurrentRate
	} else {
		principalValue = l.OriginalPrincipal
		rateValue = l.InitialRate
	}
	remaining := l.TermMonths - elapsed
	if remaining < 1 {
		return decimal.Decimal{}, decimal.Decimal{}, 0, errs.New(errs.KindUsage,
			"el plazo del préstamo ya está agotado en la fecha de partida: no quedan cuotas que calcular", "")
	}
	principal, err := model.NewDecimal(principalValue)
	if err != nil {
		return decimal.Decimal{}, decimal.Decimal{}, 0, errs.New(errs.KindUsage,
			fmt.Sprintf("el capital pendiente %q no es un decimal válido", principalValue), "")
	}
	rate, err := model.NewDecimal(rateValue)
	if err != nil {
		return decimal.Decimal{}, decimal.Decimal{}, 0, errs.New(errs.KindUsage,
			fmt.Sprintf("el tipo %q no es un decimal válido", rateValue), "")
	}
	return principal.Decimal(), rate.Decimal(), remaining, nil
}

// monthsBetween cuenta los meses transcurridos entre dos fechas, por año y
// mes (el día no entra: una cuota del día 1 y otra del día 28 del mismo mes
// son el mismo mes de cuota).
func monthsBetween(from, to time.Time) int {
	return (to.Year()*12 + int(to.Month())) - (from.Year()*12 + int(from.Month()))
}

// addMonths suma n meses a un mes (día 1) y fija el día de pago del préstamo.
func addMonths(base time.Time, n int, paymentDay int) time.Time {
	y, m, _ := base.AddDate(0, n, 0).Date()
	day := paymentDay
	if day > daysIn(y, m) {
		day = daysIn(y, m)
	}
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

// daysIn devuelve los días de un mes.
func daysIn(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// revisionMonthFromStart devuelve los meses desde el inicio en los que caen
// las revisiones del préstamo: el fin del tramo inicial y, a partir de ahí,
// cada revision_every_months. Orden ascendente; sólo dentro del plazo.
func revisionMonthFromStart(l loan.Loan) []int {
	var months []int
	if l.InitialPeriodMonths == nil {
		return months
	}
	e := *l.InitialPeriodMonths
	for e < l.TermMonths {
		months = append(months, e)
		e += l.RevisionEveryMonths
	}
	return months
}

// floorOf convierte el suelo del préstamo a decimal: nil significa sin suelo
// (floor_rate = "" declarado, decisión 1 de LOAN-01: no equivale a suelo 0).
func floorOf(l loan.Loan) (*decimal.Decimal, error) {
	if l.FloorRate == nil || *l.FloorRate == "" {
		return nil, nil
	}
	f, err := model.NewDecimal(*l.FloorRate)
	if err != nil {
		return nil, err
	}
	d := f.Decimal()
	return &d, nil
}

// rateOf es la cadena de revisión del núcleo (convención 1.5 de
// docs/calculos.md) con los datos del préstamo: índice + diferencial → suelo
// → redondeo de escritura.
func rateOf(l loan.Loan, indexValue decimal.Decimal) (finance.Rate, error) {
	spread, err := model.NewDecimal(l.Spread)
	if err != nil {
		return finance.Rate{}, errs.New(errs.KindUsage,
			fmt.Sprintf("el diferencial %q no es un decimal válido", l.Spread), "")
	}
	floor, err := floorOf(l)
	if err != nil {
		return finance.Rate{}, errs.New(errs.KindUsage,
			fmt.Sprintf("el suelo %q no es un decimal válido", *l.FloorRate), "")
	}
	return finance.NewRate(indexValue, spread.Decimal(), floor, finance.RateRounding(l.RateRounding))
}
