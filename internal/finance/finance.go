package finance

import (
	"errors"
	"math/big"

	"github.com/shopspring/decimal"
)

// Errores centinela de validación de entradas. internal/errs los mapeará a
// usage/internal en CORE-01.
//
// Un tipo de interés negativo NO es error (FIN-02, decisión 3): un préstamo
// sin suelo con índice negativo produce un TIN negativo y se amortiza con
// interés mensual negativo.
var (
	ErrNegativePrincipal = errors.New("finance: el capital no puede ser negativo")
	ErrNegativeMonths    = errors.New("finance: el plazo no puede ser negativo")
	ErrZeroMonths        = errors.New("finance: el plazo debe ser de al menos un mes")
)

var (
	ratHundred = big.NewRat(100, 1)
	ratOne     = big.NewRat(1, 1)
	ratHalf    = big.NewRat(1, 2)
	ratMonths  = big.NewRat(100*12, 1) // 100 % anual → tipo mensual en un paso
)

// validate aplica las reglas de entrada comunes a Payment y Amortization. El
// tipo puede ser negativo (decisión 3 de FIN-02): sólo se rechazan un capital
// negativo y un plazo vacío.
func validate(principal, annualRate decimal.Decimal, months int) error {
	if principal.Sign() < 0 {
		return ErrNegativePrincipal
	}
	if months < 0 {
		return ErrNegativeMonths
	}
	if months == 0 {
		return ErrZeroMonths
	}
	return nil
}

// monthlyRateRat devuelve i = TIN/100/12 como racional exacto (convención 1.1
// de docs/calculos.md, monthly-nominal).
func monthlyRateRat(annualRate decimal.Decimal) *big.Rat {
	rate := annualRate.Rat() // conversión exacta decimal → big.Rat
	return new(big.Rat).Quo(rate, ratMonths)
}

// powRat calcula base^exp con exponenciación binaria, todo en racionales exactos.
func powRat(base *big.Rat, exp int) *big.Rat {
	result := big.NewRat(1, 1)
	b := new(big.Rat).Set(base)
	for exp > 0 {
		if exp&1 == 1 {
			result.Mul(result, b)
		}
		if exp > 1 {
			b.Mul(b, b)
		}
		exp >>= 1
	}
	return result
}

// roundHalfUpCents redondea un racional a un número entero de céntimos,
// half-up lejos de cero (convención 1.3 de docs/calculos.md), simétrico para
// ambos signos: se redondea el valor absoluto hacia arriba y se restaura el
// signo, de modo que −0.005 da −0.01 y no 0.00. Con entradas no negativas el
// resultado es el de siempre: floor(x·100 + 1/2).
func roundHalfUpCents(r *big.Rat) *big.Int {
	abs := new(big.Rat).Abs(r)
	scaled := new(big.Rat).Mul(abs, ratHundred)
	shifted := new(big.Rat).Add(scaled, ratHalf)
	cents := new(big.Int).Quo(shifted.Num(), shifted.Denom())
	if r.Sign() < 0 {
		cents.Neg(cents)
	}
	return cents
}

// centsIntToDecimal convierte un número entero de céntimos en decimal exacto,
// sin pasar por float ni por la precisión de división global de shopspring.
func centsIntToDecimal(cents *big.Int) decimal.Decimal {
	return decimal.NewFromBigInt(cents, -2)
}

// Payment devuelve la cuota constante del sistema francés (convención 1.2):
//
//	C = P · i / (1 − (1 + i)^−n)
//
// con i = TIN/100/12. Con i = 0, C = P/n (sin división por cero). Con
// i < 0 (préstamo sin suelo con índice negativo, decisión 3 de FIN-02) el
// interés mensual es negativo: la amortización supera la cuota y el préstamo
// se cierra antes del mes n por la regla de cierre anticipado de
// Amortization. La cuota exacta se redondea half-up a céntimos una sola vez.
// Todo el cálculo interno es math/big.Rat (decisión D-03).
func Payment(principal, annualRate decimal.Decimal, months int) (decimal.Decimal, error) {
	if err := validate(principal, annualRate, months); err != nil {
		return decimal.Decimal{}, err
	}
	p := principal.Rat()
	i := monthlyRateRat(annualRate)

	var c *big.Rat
	if i.Sign() == 0 {
		c = new(big.Rat).Quo(p, big.NewRat(int64(months), 1))
	} else {
		growth := powRat(new(big.Rat).Add(ratOne, i), months) // (1+i)^n
		denom := new(big.Rat).Sub(ratOne, new(big.Rat).Inv(growth))
		c = new(big.Rat).Quo(new(big.Rat).Mul(p, i), denom)
	}
	return centsIntToDecimal(roundHalfUpCents(c)), nil
}
