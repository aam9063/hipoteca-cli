package finance

import (
	"fmt"
	"math/big"
	"time"

	"github.com/shopspring/decimal"
)

// ErrUnsupportedRateRounding: la escritura declara un redondeo de tipo que la
// CLI no soporta. Es error de configuración, nunca un redondeo silencioso
// (FIN-02, decisión 2).
var ErrUnsupportedRateRounding = fmt.Errorf("finance: redondeo de tipo no soportado")

// RateRounding es el redondeo de escritura del tipo (`rate_rounding` en la
// documentación de la escritura, convención 1.5 de docs/calculos.md). Los
// valores soportados son los cuatro de abajo; cualquier otro es error.
type RateRounding string

const (
	// RateRoundingNone no redondea: el TIN es índice + diferencial (y suelo).
	RateRoundingNone RateRounding = "none"
	// RateRoundingEighth redondea al múltiplo de 1/8 (0.125) más próximo.
	RateRoundingEighth RateRounding = "1/8"
	// RateRoundingQuarter redondea al múltiplo de 1/4 (0.25) más próximo.
	RateRoundingQuarter RateRounding = "1/4"
	// RateRoundingFiveHundredths redondea al múltiplo de 0.05 más próximo.
	RateRoundingFiveHundredths RateRounding = "0.05"
)

// rateRoundingMultiples mapea cada modo soportado a su múltiplo como decimal
// exacto. Un modo ausente del mapa no está soportado; nil significa sin
// redondeo (se conserva el decimal tal cual).
var rateRoundingMultiples = map[RateRounding]*decimal.Decimal{
	RateRoundingNone:           nil,
	RateRoundingEighth:         rateMultiple("0.125"),
	RateRoundingQuarter:        rateMultiple("0.25"),
	RateRoundingFiveHundredths: rateMultiple("0.05"),
}

func rateMultiple(s string) *decimal.Decimal {
	d := decimal.RequireFromString(s)
	return &d
}

// Rate es el resultado de una revisión de tipo, con cada paso de la cadena
// expuesto para que la salida pueda mostrar el porqué de la cifra (P-04):
//
//	bruto    = índice + diferencial
//	conSuelo = max(bruto, suelo)          (o bruto, si no hay suelo)
//	final    = redondeoEscritura(conSuelo) (orden normativo: el suelo va
//	                                        antes del redondeo, decisión 1)
type Rate struct {
	Bruto    decimal.Decimal
	ConSuelo decimal.Decimal
	Final    decimal.Decimal
}

// roundRateHalfUpToMultiple redondea r al múltiplo más próximo, half-up en el
// punto medio lejos de cero (simétrico para ambos signos, FIN-02: un TIN
// negativo en el punto medio redondea hacia abajo, p. ej. −3.0625 con 1/8 da
// −3.125), todo en aritmética exacta: se redondea el cociente en valor
// absoluto y se restaura el signo.
//
//	units = floor(|r|/múltiplo + 1/2) con el signo de r (en big.Rat)
//	final = units · múltiplo          (producto decimal exacto)
func roundRateHalfUpToMultiple(r decimal.Decimal, multiple decimal.Decimal) decimal.Decimal {
	quotient := new(big.Rat).Quo(r.Rat(), multiple.Rat())
	shifted := new(big.Rat).Add(new(big.Rat).Abs(quotient), ratHalf)
	// Div es división con suelo (Quo trunca hacia cero): floor exacto.
	units := new(big.Int).Quo(shifted.Num(), shifted.Denom())
	if quotient.Sign() < 0 {
		units.Neg(units)
	}
	return decimal.NewFromBigInt(units, 0).Mul(multiple)
}

// NewRate calcula el TIN aplicable de una revisión con el orden normativo de
// la convención 1.5 de docs/calculos.md: índice + diferencial → suelo →
// redondeo de escritura (half-up al múltiplo).
//
// Un floor nil significa sin suelo (decisión 3): no equivale a suelo 0, de
// modo que un índice negativo da un TIN negativo. Un rounding no soportado
// devuelve ErrUnsupportedRateRounding.
func NewRate(index, spread decimal.Decimal, floor *decimal.Decimal, rounding RateRounding) (Rate, error) {
	bruto := index.Add(spread)

	conSuelo := bruto
	if floor != nil && conSuelo.LessThan(*floor) {
		conSuelo = *floor
	}

	final := conSuelo
	multiple, ok := rateRoundingMultiples[rounding]
	if !ok {
		return Rate{}, fmt.Errorf("finance: redondeo de tipo %q: %w", string(rounding), ErrUnsupportedRateRounding)
	}
	if multiple != nil {
		final = roundRateHalfUpToMultiple(conSuelo, *multiple)
	}

	return Rate{Bruto: bruto, ConSuelo: conSuelo, Final: final}, nil
}

// ReferenceMonth devuelve el año-mes del índice que corresponde a una
// revisión: revision menos offsetMonths meses, normalizado al día 1 del mes
// en UTC. Es aritmética de meses pura (decisión 4): ni la zona horaria de
// revision ni su día ni su hora entran en el resultado.
func ReferenceMonth(revision time.Time, offsetMonths int) time.Time {
	year, month, _ := revision.Date()
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC).AddDate(0, -offsetMonths, 0)
}
