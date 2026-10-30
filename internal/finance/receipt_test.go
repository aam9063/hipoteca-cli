package finance

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/shopspring/decimal"
)

// El caso 11 de ../../testdata/finance es un recibo bancario real, no un
// cálculo de generate.py: es el único contraste del proyecto contra los
// números de un banco. Este test comprueba lo que el programa decide (el tipo
// desde índice y diferencial, y el interés del mes redondeado a céntimos) y
// las dos identidades que el propio recibo declara.
type receiptRefCase struct {
	ID     string `json:"id"`
	Inputs struct {
		Principal            string  `json:"principal"`
		TIN                  string  `json:"tin"`
		IndexValue           string  `json:"index_value"`
		IndexReferenceMonth  string  `json:"index_reference_month"`
		IndexSeries          string  `json:"index_series"`
		Spread               string  `json:"spread"`
		RateRounding         string  `json:"rate_rounding"`
		FloorRate            *string `json:"floor_rate"`
		ReferenceOffsetMonth int     `json:"reference_offset_months"`
	} `json:"inputs"`
	Expected struct {
		TIN               string `json:"tin_after_rounding"`
		Interest          string `json:"interest"`
		PrincipalRepaid   string `json:"principal_repaid"`
		Payment           string `json:"payment"`
		OutstandingBefore string `json:"outstanding_before"`
		OutstandingAfter  string `json:"outstanding_after"`
	} `json:"expected"`
}

func loadReceiptRefCase(t *testing.T) receiptRefCase {
	t.Helper()
	data, err := os.ReadFile("../../testdata/finance/case-11-recibo-real.json")
	if err != nil {
		t.Fatalf("leyendo el caso del recibo: %v", err)
	}
	var tc receiptRefCase
	if err := json.Unmarshal(data, &tc); err != nil {
		t.Fatalf("interpretando el caso del recibo: %v", err)
	}
	return tc
}

func dec(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("decimal %q: %v", s, err)
	}
	return d
}

// TestReciboRealElInteresDelMesCoincideConElBanco reproduce la cadena que el
// programa usa de verdad: índice + diferencial para el TIN, y el interés del
// mes tal como lo calcula el cuadro (balance × tipo mensual, redondeo half-up
// a céntimos). Que coincida al céntimo con el recibo es lo que valida la
// convención 1.4, no una reimplementación en el test.
func TestReciboRealElInteresDelMesCoincideConElBanco(t *testing.T) {
	tc := loadReceiptRefCase(t)

	rate, err := NewRate(dec(t, tc.Inputs.IndexValue), dec(t, tc.Inputs.Spread), nil,
		RateRounding(tc.Inputs.RateRounding))
	if err != nil {
		t.Fatalf("NewRate: %v", err)
	}
	wantTIN := dec(t, tc.Expected.TIN)
	if !rate.Final.Equal(wantTIN) {
		t.Fatalf("TIN = %s, el recibo dice %s", rate.Final, wantTIN)
	}
	// El TIN del recibo también es el dato que el caso declara como entrada:
	// si la escritura y el índice no cuadran, el test debe decirlo.
	if !dec(t, tc.Inputs.TIN).Equal(wantTIN) {
		t.Fatalf("el caso declara TIN %s y espera %s", tc.Inputs.TIN, wantTIN)
	}

	principal := dec(t, tc.Inputs.Principal)
	// Un solo mes: el interés de la primera fila es el del recibo, porque el
	// tipo mensual y el redondeo a céntimos son los mismos que en cualquier
	// otro tramo.
	schedule, err := CompoundSchedule(
		Loan{Principal: principal, AnnualRate: rate.Final, Months: 1},
		[]Period{{Rate: rate.Final, Months: 1}},
	)
	if err != nil {
		t.Fatalf("CompoundSchedule: %v", err)
	}
	wantInterest := dec(t, tc.Expected.Interest)
	if !schedule.Rows[0].Interest.Equal(wantInterest) {
		t.Fatalf("interés del mes = %s, el recibo dice %s",
			schedule.Rows[0].Interest, wantInterest)
	}

	// Y que el cálculo no se apoye en float: la comprobación con big.Rat
	// reproduce 109479.58 × 0.503 % ÷ 12 exacto y su redondeo a céntimos.
	monthly := new(big.Rat).SetFrac(
		rate.Final.Rat().Num(),
		new(big.Int).Mul(rate.Final.Rat().Denom(), big.NewInt(1200)),
	)
	exact := new(big.Rat).Mul(principal.Rat(), monthly)
	if got := centsIntToDecimal(roundHalfUpCents(exact)); !got.Equal(wantInterest) {
		t.Fatalf("interés exacto redondeado = %s, el recibo dice %s", got, wantInterest)
	}
}

// TestReciboRealLasIdentidadesDelPapel comprueba las dos relaciones que el
// propio recibo declara entre sus cinco importes. No prueban al programa:
// prueban que lo transcrito es coherente consigo mismo, para que un error de
// copia no se cuele como referencia.
func TestReciboRealLasIdentidadesDelPapel(t *testing.T) {
	tc := loadReceiptRefCase(t)

	interest := dec(t, tc.Expected.Interest)
	repaid := dec(t, tc.Expected.PrincipalRepaid)
	before := dec(t, tc.Expected.OutstandingBefore)
	after := dec(t, tc.Expected.OutstandingAfter)

	if got := interest.Add(repaid); !got.Equal(dec(t, tc.Expected.Payment)) {
		t.Fatalf("interés + amortización = %s, el recibo dice cuota %s", got, tc.Expected.Payment)
	}
	if got := before.Sub(repaid); !got.Equal(after) {
		t.Fatalf("capital antes − amortización = %s, el recibo dice %s", got, after)
	}
}
