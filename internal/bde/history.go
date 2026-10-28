package bde

import (
	"fmt"
	"strconv"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
)

// nowYear responde el año en curso. Es una variable para que los tests fijen el
// "hoy" y la validación de años futuros sea determinista.
var nowYear = func() int { return time.Now().Year() }

// Rango es el parámetro `rango` de listaSeries (docs/series.md §2.3). La API
// sólo acepta 30M, 60M, MAX y un año con datos; rechaza 12M, 24M, 36M, 120M y
// años futuros con 412. El tipo expone exactamente esas cuatro formas por
// construcción (decisión 2): un `--range` arbitrario de la CLI se resuelve en
// CMD-01 pidiendo MAX o el año y recortando en cliente.
type Rango struct{ value string }

// String devuelve el valor literal que viaja en la petición.
func (r Rango) String() string { return r.value }

// Los tres rangos fijos que la API acepta (docs/series.md §2.3).
var (
	Rango30M = Rango{"30M"} // 31 puntos: 30 meses + el mes en curso
	Rango60M = Rango{"60M"} // 61 puntos
	RangoMAX = Rango{"MAX"} // toda la serie
)

// RangoAnio construye el rango de un año. Sólo se exponen años desde 1990
// hasta el año en curso: la API rechaza años futuros con 412, así que no se
// dejan construir (decisión 2). Que el año tenga datos publicados es cosa de
// cada serie: un año sin datos llega simplemente vacío.
func RangoAnio(year int) (Rango, error) {
	if year < 1990 || year > nowYear() {
		return Rango{}, errs.New(errs.KindUsage,
			fmt.Sprintf("el rango %d no es correcto: el año debe estar entre 1990 y el año en curso", year),
			"usa Rango30M, Rango60M, RangoMAX o un año con datos publicados")
	}
	return Rango{strconv.Itoa(year)}, nil
}

// year informa de si el rango es un año (`2025`) y cuál.
func (r Rango) year() (int, bool) {
	y, err := strconv.Atoi(r.value)
	if err != nil {
		return 0, false
	}
	return y, true
}

// Observation es un valor publicado de una serie a granularidad de mes.
// La API da la fecha como instante ISO 8601 pero la hora no es significativa ni
// estable (docs/series.md §5): Month está normalizado al día 1 del mes a las
// 00:00 UTC, y todo lo que compara meses debe usar esa normalización.
type Observation struct {
	// Code es el código de la serie del Banco de España (p. ej. D_1NBAF472).
	Code string
	// Month es el mes del valor, normalizado a su día 1, 00:00 UTC.
	Month time.Time
	// Value es el valor exacto tal como lo publicó el Banco (P-07).
	Value model.Decimal
}

// Series es un histórico mensual de una serie, con sus metadatos de
// publicación. Observations va SIEMPRE en orden ascendente: la API lo devuelve
// descendente y el cliente lo invierte una vez, aquí (docs/series.md §3.2).
type Series struct {
	Code             string
	Description      string
	DescriptionShort string
	Frequency        string    // codFrecuencia de la respuesta ("M")
	Decimals         int       // decimales que publica el Banco para esta serie
	Unit             string    // `simbolo`; puede faltar en una respuesta válida
	Start            time.Time // fechaInicio de la serie completa, normalizada a mes
	End              time.Time // fechaFin de la serie completa, normalizada a mes
	Observations     []Observation
}
