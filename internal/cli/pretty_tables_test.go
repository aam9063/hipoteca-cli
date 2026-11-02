package cli

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/loan"
)

// ----------------------------------------------------------------------------
// S7 de PRETTY-01: un test por tabla --pretty (index list, revision, amortize,
// schedule y loan list). Cada test comprueba cabeceras, alineación, que el
// número de filas coincide con el JSON y que sin --pretty la salida sigue
// siendo un único valor JSON (S2). No simula un TTY: el ancho es fijo (S3).
// ----------------------------------------------------------------------------

// sinSaltos aplana la salida a una sola línea: las líneas de resumen envuelven
// por palabras, y comparar frases largas (la atribución) exige ignorar dónde
// cayó el salto.
func sinSaltos(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// columnStarts devuelve la posición (en runas) donde empieza cada columna
// dentro de una línea de la tabla.
func columnStarts(cols []prettyCol) []int {
	starts := make([]int, len(cols))
	pos := len(prettyIndent)
	for i, c := range cols {
		starts[i] = pos
		pos += c.width + prettyGap
	}
	return starts
}

// assertPrettyTabla comprueba lo común de toda tabla --pretty: las cabeceras
// están, la cabecera y la primera fila arrancan cada celda en su columna
// (alineación) y ninguna línea pasa del ancho fijo (S3).
func assertPrettyTabla(t *testing.T, out string, cols []prettyCol) {
	t.Helper()
	for _, c := range cols {
		if !strings.Contains(out, c.header) {
			t.Errorf("falta la cabecera %q en la tabla:\n%s", c.header, out)
		}
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for _, line := range lines {
		if n := len([]rune(line)); n > prettyWidth+len(prettyIndent) {
			t.Errorf("línea de %d runas, el máximo es %d: %q", n, prettyWidth+len(prettyIndent), line)
		}
	}
	header := -1
	for i, line := range lines {
		if strings.Contains(line, cols[0].header) {
			header = i
			break
		}
	}
	if header < 0 {
		t.Fatalf("no se ha encontrado la línea de cabecera:\n%s", out)
	}
	// La cabecera y la primera fila de datos llevan todas las columnas.
	for _, line := range []string{lines[header], lines[header+1]} {
		for i, start := range columnStarts(cols) {
			runes := []rune(line)
			if start >= len(runes) || runes[start] == ' ' {
				t.Errorf("la columna %d (%s) no empieza donde toca: %q", i, cols[i].header, line)
			}
		}
	}
}

// runPretty ejecuta el comando con --pretty y devuelve su stdout.
func runPretty(t *testing.T, deps Deps, args ...string) string {
	t.Helper()
	stdout, stderr, err := runRoot(t, deps, append(args, "--pretty")...)
	if err != nil {
		t.Fatalf("%v --pretty: %v (stderr: %s)", args, err, stderr)
	}
	return stdout
}

// obsParaTodoElCatalogo devuelve una observación por índice del catálogo,
// todas del mismo mes: la fuente stub las sirve sin red.
func obsParaTodoElCatalogo(t *testing.T, cat *bde.Catalog, mes time.Time, valor string) []bde.Observation {
	t.Helper()
	v := mustDecimal(t, valor)
	ids := cat.IDs()
	obs := make([]bde.Observation, 0, len(ids))
	for _, id := range ids {
		idx, err := cat.Lookup(id)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		obs = append(obs, bde.Observation{Code: idx.Code, Month: mes, Value: v})
	}
	return obs
}

// TestIndexListPrettyEsUnaTabla: la tabla del catálogo con el último valor de
// cada índice; el número de filas coincide con el JSON.
func TestIndexListPrettyEsUnaTabla(t *testing.T) {
	sept2026 := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	src := &stubSource{latest: obsParaTodoElCatalogo(t, catDeTest(t), sept2026, "3.247")}
	deps := depsCon(t, src)

	stdout, _, err := runRoot(t, deps, "index", "list")
	if err != nil {
		t.Fatalf("index list: %v", err)
	}
	assertSingleJSON(t, stdout)
	var res app.IndexValues
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("la salida por defecto debe ser un único valor JSON: %v", err)
	}
	if len(res.Indices) == 0 {
		t.Fatal("el JSON debe traer los índices del catálogo")
	}

	out := runPretty(t, deps, "index", "list")
	cols := indexPrettyCols()
	assertPrettyTabla(t, out, cols)
	// Una fila por índice del JSON, cada id encabezando su línea.
	for _, v := range res.Indices {
		if !strings.Contains(out, "\n  "+v.ID) {
			t.Errorf("el índice %q no encabeza ninguna fila:\n%s", v.ID, out)
		}
	}
	// Formato es-ES del valor y del mes.
	if !strings.Contains(out, "3,247 %") || !strings.Contains(out, "2026-09") {
		t.Errorf("faltan el valor y el mes en formato es-ES:\n%s", out)
	}
	// La atribución del Banco de España bajo la tabla.
	if !strings.Contains(sinSaltos(out), "Banco de España") {
		t.Errorf("falta la atribución bajo la tabla:\n%s", out)
	}
}

func catDeTest(t *testing.T) *bde.Catalog {
	t.Helper()
	cat, err := bde.LoadCatalog(nil)
	if err != nil {
		t.Fatalf("cargar el catálogo: %v", err)
	}
	return cat
}

// TestRevisionPrettyEsUnaTabla: la revisión en filas campo-valor, con su
// línea de resumen (S5): mes del índice y atribución del Banco.
func TestRevisionPrettyEsUnaTabla(t *testing.T) {
	src := serieFixture(t)
	deps := depsConConfig(t, &src, seedLoan(t, "casa"))

	stdout, _, err := runRoot(t, deps, "revision", "casa", "--date", "2026-11")
	if err != nil {
		t.Fatalf("revision: %v", err)
	}
	assertSingleJSON(t, stdout)
	var res app.RevisionResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("la salida por defecto debe ser un único valor JSON: %v", err)
	}

	out := runPretty(t, deps, "revision", "casa", "--date", "2026-11")
	cols := revisionPrettyCols()
	assertPrettyTabla(t, out, cols)
	// Cada campo del JSON está en la tabla.
	etiquetas := []string{"Préstamo", "Mes de referencia", "Valor del índice",
		"TIN anterior", "TIN nuevo", "Capital pendiente", "Cuota anterior", "Cuota nueva"}
	for _, e := range etiquetas {
		if !strings.Contains(out, e) {
			t.Errorf("falta la fila %q en la tabla:\n%s", e, out)
		}
	}
	// Formato es-ES: tipo con coma y %, importes con coma y €.
	if !strings.Contains(out, "3,247 %") {
		t.Errorf("el valor del índice debe salir como 3,247 %%:\n%s", out)
	}
	if !strings.Contains(out, "152341,18 €") {
		t.Errorf("el capital pendiente debe salir como 152341,18 €:\n%s", out)
	}
	// Línea de resumen (S5): mes del índice y atribución exacta.
	if !strings.Contains(out, "Mes de referencia del índice: "+res.Index.ReferenceMonth) {
		t.Errorf("el resumen debe llevar el mes del índice:\n%s", out)
	}
	if !strings.Contains(sinSaltos(out), res.Attribution.Text) || !strings.Contains(out, res.Attribution.DataUpdatedAt) {
		t.Errorf("el resumen debe llevar la atribución del Banco:\n%s", out)
	}
}

// TestAmortizePrettyEsUnaTabla: en compare, las dos opciones en columnas y
// ninguna recomendación; el resumen lleva el modo y el ahorro (S5).
func TestAmortizePrettyEsUnaTabla(t *testing.T) {
	deps := depsConConfig(t, &stubSource{}, nil)
	flags := []string{"amortize", "--principal", "150000", "--rate", "3.10",
		"--months", "300", "--amount", "10000", "--date", "60", "--mode", "compare"}

	stdout, _, err := runRoot(t, deps, flags...)
	if err != nil {
		t.Fatalf("amortize: %v", err)
	}
	assertSingleJSON(t, stdout)
	var res app.AmortizeResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("la salida por defecto debe ser un único valor JSON: %v", err)
	}
	if res.ReducePayment == nil || res.ReduceTerm == nil {
		t.Fatalf("compare debe traer los dos bloques: %+v", res)
	}

	out := runPretty(t, deps, flags...)
	cols := amortizePrettyCols(res)
	assertPrettyTabla(t, out, cols)
	for _, e := range []string{"Importe amortizado", "Cuota antes", "Cuota después",
		"Intereses ahorrados", "Meses ahorrados"} {
		if !strings.Contains(out, e) {
			t.Errorf("falta la fila %q en la tabla:\n%s", e, out)
		}
	}
	// Formato es-ES de los importes de los dos bloques.
	if !strings.Contains(out, prettyMoney(res.ReducePayment.Amount)) ||
		!strings.Contains(out, prettyMoney(res.ReduceTerm.Amount)) {
		t.Errorf("faltan los importes de los dos bloques:\n%s", out)
	}
	// Resumen (S5): modo y ahorro de cada opción, sin recomendar.
	if !strings.Contains(out, "Modo: compare") ||
		!strings.Contains(out, prettyMoney(res.ReducePayment.InterestSaved)) ||
		!strings.Contains(out, prettyMoney(res.ReduceTerm.InterestSaved)) {
		t.Errorf("el resumen debe llevar el modo y el ahorro:\n%s", out)
	}
	if !strings.Contains(sinSaltos(out), "Banco de España") {
		t.Errorf("falta la atribución bajo la tabla:\n%s", out)
	}
}

// TestSchedulePrettyEsUnaTabla: una fila por cuota del JSON y el resumen con
// el número de filas y el capital final (S5).
func TestSchedulePrettyEsUnaTabla(t *testing.T) {
	src := serieFixture(t)
	deps := depsConConfig(t, &src, seedLoan(t, "casa"))

	stdout, _, err := runRoot(t, deps, "schedule", "casa")
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	assertSingleJSON(t, stdout)
	var res app.ScheduleResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("la salida por defecto debe ser un único valor JSON: %v", err)
	}
	if len(res.Rows) == 0 {
		t.Fatal("el JSON debe traer las cuotas")
	}

	out := runPretty(t, deps, "schedule", "casa")
	cols := schedulePrettyCols()
	assertPrettyTabla(t, out, cols)
	// Una línea por cuota del JSON: cada fila lleva su fecha en la FECHA.
	filas := regexp.MustCompile(`(?m)^ {2}\d+ +\d{4}-\d{2}-\d{2}`).FindAllString(out, -1)
	if len(filas) != len(res.Rows) {
		t.Errorf("la tabla muestra %d filas y el JSON trae %d", len(filas), len(res.Rows))
	}
	// Resumen (S5): número de filas y capital final, los dos en euros es-ES.
	ultima := res.Rows[len(res.Rows)-1]
	if !strings.Contains(out, prettyMoney(ultima.Balance)) {
		t.Errorf("el resumen debe llevar el capital final:\n%s", out)
	}
	if !strings.Contains(sinSaltos(out), "Banco de España") {
		t.Errorf("falta la atribución bajo la tabla:\n%s", out)
	}
}

// TestLoanListPrettyEsUnaTabla: un préstamo completo y otro incompleto, con
// la marca sí/no igual que en el JSON.
func TestLoanListPrettyEsUnaTabla(t *testing.T) {
	deps := depsConConfig(t, &stubSource{}, seedDosPrestamos)

	stdout, _, err := runRoot(t, deps, "loan", "list")
	if err != nil {
		t.Fatalf("loan list: %v", err)
	}
	assertSingleJSON(t, stdout)
	var res app.LoanListResult
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("la salida por defecto debe ser un único valor JSON: %v", err)
	}
	if len(res.Loans) != 2 {
		t.Fatalf("el JSON debe traer los dos préstamos: %+v", res.Loans)
	}

	out := runPretty(t, deps, "loan", "list")
	cols := loanListPrettyCols()
	assertPrettyTabla(t, out, cols)
	// Una fila por préstamo del JSON.
	for _, l := range res.Loans {
		if !strings.Contains(out, "\n  "+l.ID) {
			t.Errorf("el préstamo %q no encabeza ninguna fila:\n%s", l.ID, out)
		}
	}
	// La marca de completo coincide con el JSON.
	for _, l := range res.Loans {
		want := "no"
		if l.Complete {
			want = "sí"
		}
		if !strings.Contains(out, "\n  "+l.ID) {
			continue
		}
		if !filaDiceCompleto(out, l.ID, want) {
			t.Errorf("el préstamo %q (complete=%v) debe marcar %q:\n%s", l.ID, l.Complete, want, out)
		}
	}
}

// filaDiceCompleto comprueba que la línea que encabeza el préstamo id lleva
// la marca de completo en la última columna.
func filaDiceCompleto(out, id, want string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "  "+id) {
			return strings.HasSuffix(strings.TrimRight(line, " "), want)
		}
	}
	return false
}

// seedDosPrestamos escribe un préstamo completo y otro incompleto (le faltan
// campos de la escritura) en la config temporal.
func seedDosPrestamos(path string) {
	cfg := config.Default()
	piso := loan.Loan{
		Name:              "Piso en la playa",
		Type:              loan.TypeVariable,
		StartDate:         "2023-01-01",
		OriginalPrincipal: "90000",
		TermMonths:        240,
		PaymentDay:        1,
		InitialRate:       "2.0",
		Index:             "euribor-1y",
		Spread:            "0.5",
	}
	cfg.Loans = map[string]loan.Loan{"casa": loanCompletaParaE2E(), "piso": piso}
	if err := cfg.Save(path); err != nil {
		panic("setup: " + err.Error())
	}
}

// TestTruncateCellRecortaConElipsis (S6): la celda que no cabe se recorta con
// una elipsis en vez de empujar la columna.
func TestTruncateCellRecortaConElipsis(t *testing.T) {
	got := truncateCell("deuda-publica-2a6", 12)
	if got != "deuda-publi…" {
		t.Fatalf("truncateCell = %q, want %q", got, "deuda-publi…")
	}
	if truncateCell("corto", 12) != "corto" {
		t.Fatal("lo que cabe no se toca")
	}
}
