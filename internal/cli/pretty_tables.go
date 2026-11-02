package cli

import (
	"fmt"
	"strconv"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// Este fichero monta las tablas --pretty de los comandos de cálculo
// (PRETTY-01): la vista para humanos de lo mismo que cada comando publica en
// JSON. El JSON sigue siendo el defecto (S2); la tabla es opt-in, de ancho
// fijo (S3), en formato es-ES (S4) y con su línea de resumen (S5).
//
// Ninguna tabla lee del terminal ni del reloj: los anchos son constantes y el
// contenido viene del resultado de app, así que la salida es idéntica en
// cualquier máquina y en los tests.

// indexPrettyCols: el id manda (19 acoge el id más largo del catálogo), el
// nombre es la única columna de texto libre.
func indexPrettyCols() []prettyCol {
	return []prettyCol{
		{header: "ID", width: 19},
		{header: "ÍNDICE", width: 30, wrap: true},
		{header: "MES", width: 7},
		{header: "VALOR", width: 10},
	}
}

// renderIndexValuesTable: la tabla de `index list` (y la forma compartida de
// `index latest`), con el último valor de cada índice y la atribución del
// Banco debajo.
func renderIndexValuesTable(res app.IndexValues) string {
	if len(res.Indices) == 0 {
		return "No hay ningún índice en el catálogo.\n"
	}
	rows := make([][]string, 0, len(res.Indices))
	for _, v := range res.Indices {
		rows = append(rows, []string{v.ID, v.Name, v.Month, prettyPct(v.Value)})
	}
	return renderPrettyTable(indexPrettyCols(), rows,
		[]string{prettyAttributionLine(res.Attribution)})
}

// revisionPrettyCols: filas campo-valor; el valor es texto libre (la
// atribución y los avisos envuelven) y el concepto queda fijo.
func revisionPrettyCols() []prettyCol {
	return []prettyCol{
		{header: "CONCEPTO", width: 20},
		{header: "VALOR", width: 50, wrap: true},
	}
}

// renderRevisionTable: la revisión en filas campo-valor. Debajo, la línea de
// resumen (S5) con el mes del índice y la atribución del Banco, y los avisos
// del resultado: sin ellos la tabla no se entiende sola.
func renderRevisionTable(res app.RevisionResult) string {
	valorIndice := prettyPct(res.Index.Value)
	if res.Index.Status == "assumed" {
		valorIndice += " (supuesto: no es un dato publicado)"
	}
	rows := [][]string{
		{"Préstamo", res.Loan},
		{"Fecha de revisión", res.RevisionDate},
		{"Índice", res.Index.ID},
		{"Mes de referencia", res.Index.ReferenceMonth},
		{"Valor del índice", valorIndice},
		{"TIN anterior", prettyPct(res.Rate.Previous)},
		{"Diferencial", prettyPct(res.Rate.Spread)},
		{"TIN nuevo", prettyPct(res.Rate.New)},
		{"Capital pendiente", prettyMoney(res.Inputs.OutstandingPrincipal)},
		{"Meses restantes", strconv.Itoa(res.Inputs.RemainingMonths)},
		{"Cuota anterior", prettyMoney(res.Payment.Previous)},
		{"Cuota nueva", prettyMoney(res.Payment.New)},
		{"Diferencia al mes", prettyMoney(res.Payment.DifferenceMonth)},
		{"Diferencia al año", prettyMoney(res.Payment.DifferenceYear)},
	}
	summaries := []string{
		"Mes de referencia del índice: " + res.Index.ReferenceMonth,
		prettyAttributionLine(res.Attribution),
	}
	summaries = append(summaries, res.Warnings...)
	return renderPrettyTable(revisionPrettyCols(), rows, summaries)
}

// amortizePrettyCols: en compare, las dos opciones en columnas al mismo
// nivel (P-08: nada de jerarquía ni recomendación); con un modo, una sola.
// El concepto mide 22 para que el etiquetado más largo ("Plazo después
// (meses)") no parta en dos líneas.
func amortizePrettyCols(res app.AmortizeResult) []prettyCol {
	if len(amortizeBloques(res)) == 2 {
		return []prettyCol{
			{header: "CONCEPTO", width: 22, wrap: true},
			{header: "REDUCIR CUOTA", width: 23},
			{header: "REDUCIR PLAZO", width: 23},
		}
	}
	return []prettyCol{
		{header: "CONCEPTO", width: 20, wrap: true},
		{header: "VALOR", width: 50},
	}
}

// amortizeBloques devuelve las opciones presentes en el resultado, en el
// orden del contrato: primero reducir cuota, luego reducir plazo.
type amortizeBloque struct {
	nombre string
	out    *app.AmortizeOutcome
}

func amortizeBloques(res app.AmortizeResult) []amortizeBloque {
	bloques := make([]amortizeBloque, 0, 2)
	if res.ReducePayment != nil {
		bloques = append(bloques, amortizeBloque{"reducir cuota", res.ReducePayment})
	}
	if res.ReduceTerm != nil {
		bloques = append(bloques, amortizeBloque{"reducir plazo", res.ReduceTerm})
	}
	return bloques
}

// renderAmortizeTable: las cifras de cada opción una debajo de otra, y el
// resumen (S5) con el modo y el ahorro de intereses de cada opción, sin
// recomendar ninguna.
func renderAmortizeTable(res app.AmortizeResult) string {
	bloques := amortizeBloques(res)
	conceptos := []struct {
		label string
		value func(app.AmortizeOutcome) string
	}{
		{"Importe amortizado", func(o app.AmortizeOutcome) string { return prettyMoney(o.Amount) }},
		{"Comisión", func(o app.AmortizeOutcome) string { return prettyMoney(o.Fee) }},
		{"Desembolso total", func(o app.AmortizeOutcome) string { return prettyMoney(o.Outlay) }},
		{"Cuota antes", func(o app.AmortizeOutcome) string { return prettyMoney(o.PaymentBefore) }},
		{"Cuota después", func(o app.AmortizeOutcome) string { return prettyMoney(o.PaymentAfter) }},
		{"Plazo antes (meses)", func(o app.AmortizeOutcome) string { return strconv.Itoa(o.TermBefore) }},
		{"Plazo después (meses)", func(o app.AmortizeOutcome) string { return strconv.Itoa(o.TermAfter) }},
		{"Intereses antes", func(o app.AmortizeOutcome) string { return prettyMoney(o.InterestBefore) }},
		{"Intereses después", func(o app.AmortizeOutcome) string { return prettyMoney(o.InterestAfter) }},
		{"Intereses ahorrados", func(o app.AmortizeOutcome) string { return prettyMoney(o.InterestSaved) }},
		{"Meses ahorrados", func(o app.AmortizeOutcome) string { return strconv.Itoa(o.MonthsSaved) }},
	}
	rows := make([][]string, 0, len(conceptos))
	for _, c := range conceptos {
		row := make([]string, 0, len(bloques)+1)
		row = append(row, c.label)
		for _, b := range bloques {
			row = append(row, c.value(*b.out))
		}
		rows = append(rows, row)
	}

	summaries := []string{resumenAmortize(res, bloques)}
	for _, b := range bloques {
		if b.out.ClosesAtEvent != nil {
			summaries = append(summaries, fmt.Sprintf(
				"En %s el importe cierra el préstamo en el mes del evento (cuota %d): después no hay ninguna cuota.",
				b.nombre, b.out.ClosesAtEvent.AfterMonth))
		}
	}
	summaries = append(summaries, prettyAttributionLine(res.Attribution))
	return renderPrettyTable(amortizePrettyCols(res), rows, summaries)
}

// resumenAmortize es la línea de resumen de S5: el modo y el ahorro de
// intereses. En compare lleva los dos ahorros, sin recomendar.
func resumenAmortize(res app.AmortizeResult, bloques []amortizeBloque) string {
	if len(bloques) == 2 {
		return fmt.Sprintf("Modo: compare · Intereses ahorrados: %s reduciendo cuota / %s reduciendo plazo",
			prettyMoney(res.ReducePayment.InterestSaved), prettyMoney(res.ReduceTerm.InterestSaved))
	}
	b := bloques[0].out
	return fmt.Sprintf("Modo: %s · Intereses ahorrados: %s · Meses ahorrados: %d",
		res.Inputs.Mode, prettyMoney(b.InterestSaved), b.MonthsSaved)
}

// schedulePrettyCols: las seis columnas del cuadro, todas numéricas o de
// fecha; con los huecos suman exactamente el ancho fijo.
func schedulePrettyCols() []prettyCol {
	return []prettyCol{
		{header: "MES", width: 4},
		{header: "FECHA", width: 10},
		{header: "CUOTA", width: 12},
		{header: "INTERÉS", width: 12},
		{header: "CAPITAL", width: 12},
		{header: "PENDIENTE", width: 12},
	}
}

// renderScheduleTable: una fila por cuota mostrada y el resumen (S5) con el
// número de filas y el capital pendiente al final, más los totales del cuadro
// completo y los avisos del resultado.
func renderScheduleTable(res app.ScheduleResult) string {
	rows := make([][]string, 0, len(res.Rows))
	for _, r := range res.Rows {
		rows = append(rows, []string{
			strconv.Itoa(r.Month), r.Date,
			prettyMoney(r.Payment), prettyMoney(r.Interest),
			prettyMoney(r.Principal), prettyMoney(r.Balance),
		})
	}
	final := model.Decimal{}
	if len(res.Rows) > 0 {
		final = res.Rows[len(res.Rows)-1].Balance
	}
	summaries := []string{
		fmt.Sprintf("%d cuotas mostradas · capital pendiente al final: %s", len(res.Rows), prettyMoney(final)),
		fmt.Sprintf("Intereses totales del cuadro: %s · Total pagado: %s",
			prettyMoney(res.TotalInterest), prettyMoney(res.TotalPaid)),
		prettyAttributionLine(res.Attribution),
	}
	summaries = append(summaries, res.Warnings...)
	return renderPrettyTable(schedulePrettyCols(), rows, summaries)
}

// loanListPrettyCols: el nombre es la única columna de texto libre.
func loanListPrettyCols() []prettyCol {
	return []prettyCol{
		{header: "ID", width: 12},
		{header: "NOMBRE", width: 36, wrap: true},
		{header: "TIPO", width: 8},
		{header: "COMPLETO", width: 10},
	}
}

// renderLoanListTable: los préstamos guardados con su marca de completo,
// igual que en el JSON pero legible.
func renderLoanListTable(res app.LoanListResult) string {
	if len(res.Loans) == 0 {
		return "No hay ningún préstamo guardado.\n"
	}
	rows := make([][]string, 0, len(res.Loans))
	for _, l := range res.Loans {
		completo := "no"
		if l.Complete {
			completo = "sí"
		}
		rows = append(rows, []string{l.ID, l.Name, l.Type, completo})
	}
	return renderPrettyTable(loanListPrettyCols(), rows,
		[]string{fmt.Sprintf("%d préstamos guardados · \"hipoteca loan show <id>\" enseña los campos de la escritura que falten", len(res.Loans))})
}
