package finance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// Las revisiones de los casos mixtos (6 a 8) viven en ../../testdata/finance y
// son la fuente de verdad: este test nunca escribe los TIN esperados a mano
// (arranque §9).
type refRevision struct {
	RevisionNumber   int    `json:"revision_number"`
	FirstMonth       int    `json:"first_month"`
	ReferenceMonth   string `json:"reference_month"`
	IndexValue       string `json:"index_value"`
	TinRaw           string `json:"tin_raw"`
	TinAfterFloor    string `json:"tin_after_floor"`
	TinAfterRounding string `json:"tin_after_rounding"`
	RemainingMonths  int    `json:"remaining_months"`
	Payment          string `json:"payment"`
}

type refMixedCase struct {
	ID     string `json:"id"`
	Inputs struct {
		Principal             string  `json:"principal"`
		TermMonths            int     `json:"term_months"`
		LoanStart             string  `json:"loan_start"`
		InitialRate           string  `json:"initial_rate"`
		InitialPeriodMonths   int     `json:"initial_period_months"`
		ReferenceOffsetMonths int     `json:"reference_offset_months"`
		Spread                string  `json:"spread"`
		FloorRate             *string `json:"floor_rate"`
		RateRounding          string  `json:"rate_rounding"`
	} `json:"inputs"`
	Expected struct {
		Payment                    string        `json:"payment"`
		TotalInterest              string        `json:"total_interest"`
		TotalPaid                  string        `json:"total_paid"`
		FirstRow                   refRow        `json:"first_row"`
		LastRow                    refRow        `json:"last_row"`
		SumPrincipalRepaid         string        `json:"sum_principal_repaid"`
		FinalOutstanding           string        `json:"final_outstanding"`
		Revisions                  []refRevision `json:"revisions"`
		FirstRowAfterFirstRevision refRow        `json:"first_row_after_first_revision"`
	} `json:"expected"`
}

func loadMixedCase(t *testing.T, name string) refMixedCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(testdataDir, name))
	if err != nil {
		t.Fatalf("leer %s: %v", name, err)
	}
	var c refMixedCase
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("parsear %s: %v", name, err)
	}
	return c
}

var mixedCaseFiles = []string{
	"case-06-mixta-12m.json",
	"case-07-suelo.json",
	"case-08-redondeo-octavo.json",
}

// TestNewRateReferenceCases comprueba la cadena bruto → conSuelo → final de
// cada revisión de los casos 6, 7 y 8 contra los TIN del JSON: ni el suelo ni
// el redondeo se suponen, se leen del caso.
func TestNewRateReferenceCases(t *testing.T) {
	for _, name := range mixedCaseFiles {
		c := loadMixedCase(t, name)
		var floor *decimal.Decimal
		if c.Inputs.FloorRate != nil {
			f := mustDecimal(t, *c.Inputs.FloorRate)
			floor = &f
		}
		spread := mustDecimal(t, c.Inputs.Spread)
		rounding := RateRounding(c.Inputs.RateRounding)

		for _, rev := range c.Expected.Revisions {
			t.Run(fmt.Sprintf("%s/revision-%d", c.ID, rev.RevisionNumber), func(t *testing.T) {
				rate, err := NewRate(mustDecimal(t, rev.IndexValue), spread, floor, rounding)
				if err != nil {
					t.Fatalf("NewRate(%s, %s, suelo, %s): %v", rev.IndexValue, spread.String(), rounding, err)
				}
				if !rate.Bruto.Equal(mustDecimal(t, rev.TinRaw)) {
					t.Errorf("bruto = %s, se esperaba %s (tin_raw)", rate.Bruto.String(), rev.TinRaw)
				}
				if !rate.ConSuelo.Equal(mustDecimal(t, rev.TinAfterFloor)) {
					t.Errorf("conSuelo = %s, se esperaba %s (tin_after_floor)", rate.ConSuelo.String(), rev.TinAfterFloor)
				}
				if !rate.Final.Equal(mustDecimal(t, rev.TinAfterRounding)) {
					t.Errorf("final = %s, se esperaba %s (tin_after_rounding)", rate.Final.String(), rev.TinAfterRounding)
				}
			})
		}
	}
}

// TestRateRoundingHalfUp fija el half-up al múltiplo en el punto medio exacto
// para cada modo soportado, más un valor que se queda por debajo. El half-up
// es lejos de cero (simétrico para ambos signos, FIN-02): −0.0625 con 1/8 da
// −0.125, no 0.
func TestRateRoundingHalfUp(t *testing.T) {
	tests := []struct {
		rounding RateRounding
		input    string
		want     string
	}{
		{RateRoundingNone, "1.234567", "1.234567"},
		{RateRoundingNone, "0.618", "0.618"},
		{RateRoundingEighth, "0.0625", "0.125"},   // punto medio exacto → arriba
		{RateRoundingEighth, "-0.0625", "-0.125"}, // punto medio exacto → lejos de cero
		{RateRoundingEighth, "-3.0625", "-3.125"}, // punto medio exacto negativo
		{RateRoundingEighth, "-0.06", "0"},
		{RateRoundingEighth, "-0.07", "-0.125"},
		{RateRoundingEighth, "0.06", "0"},
		{RateRoundingEighth, "0.1875", "0.25"},
		{RateRoundingQuarter, "0.125", "0.25"},   // punto medio exacto → arriba
		{RateRoundingQuarter, "-0.125", "-0.25"}, // punto medio exacto → lejos de cero
		{RateRoundingQuarter, "0.124", "0"},
		{RateRoundingQuarter, "0.375", "0.5"},
		{RateRoundingFiveHundredths, "0.025", "0.05"},   // punto medio exacto → arriba
		{RateRoundingFiveHundredths, "-0.025", "-0.05"}, // punto medio exacto → lejos de cero
		{RateRoundingFiveHundredths, "0.024", "0"},
		{RateRoundingFiveHundredths, "-0.024", "0"},
		{RateRoundingFiveHundredths, "0.075", "0.1"},
	}
	for _, tc := range tests {
		t.Run(string(tc.rounding)+"·"+tc.input, func(t *testing.T) {
			// Diferencial 0 y sin suelo: la cadena no toca el índice salvo el
			// redondeo de escritura.
			rate, err := NewRate(mustDecimal(t, tc.input), decimal.Zero, nil, tc.rounding)
			if err != nil {
				t.Fatalf("NewRate(%s, 0, nil, %s): %v", tc.input, tc.rounding, err)
			}
			if !rate.Final.Equal(mustDecimal(t, tc.want)) {
				t.Errorf("final = %s, se esperaba %s", rate.Final.String(), tc.want)
			}
		})
	}
}

// TestNewRateUnsupportedRounding: un redondeo no soportado es error de
// configuración, nunca un redondeo silencioso (decisión 2).
func TestNewRateUnsupportedRounding(t *testing.T) {
	for _, bad := range []string{"", "1/16", "1/2", "0.01", "0.10", "NONE", "None"} {
		t.Run("redondeo "+bad, func(t *testing.T) {
			_, err := NewRate(mustDecimal(t, "2"), decimal.Zero, nil, RateRounding(bad))
			if !errors.Is(err, ErrUnsupportedRateRounding) {
				t.Fatalf("redondeo %q: se esperaba ErrUnsupportedRateRounding, se obtuvo %v", bad, err)
			}
		})
	}
}

// TestNewRateNilFloorIsNoFloor: sin suelo no hay max(bruto, 0); un índice
// negativo con suelo vacío da un TIN negativo (decisión 3).
func TestNewRateNilFloorIsNoFloor(t *testing.T) {
	rate, err := NewRate(mustDecimal(t, "-0.5"), mustDecimal(t, "0.2"), nil, RateRoundingNone)
	if err != nil {
		t.Fatalf("NewRate(-0.5, 0.2, nil, none): %v", err)
	}
	want := mustDecimal(t, "-0.3")
	if !rate.Bruto.Equal(want) || !rate.ConSuelo.Equal(want) || !rate.Final.Equal(want) {
		t.Errorf("cadena = (%s, %s, %s), se esperaba (-0.3, -0.3, -0.3)",
			rate.Bruto.String(), rate.ConSuelo.String(), rate.Final.String())
	}

	// Contraste: un suelo explícito de 0 sí trunca a 0. No es lo mismo.
	zero := mustDecimal(t, "0")
	rate0, err := NewRate(mustDecimal(t, "-0.5"), mustDecimal(t, "0.2"), &zero, RateRoundingNone)
	if err != nil {
		t.Fatalf("NewRate(-0.5, 0.2, 0, none): %v", err)
	}
	if !rate0.ConSuelo.IsZero() || !rate0.Final.IsZero() {
		t.Errorf("con suelo 0: conSuelo = %s, final = %s, se esperaba 0",
			rate0.ConSuelo.String(), rate0.Final.String())
	}
}

// TestReferenceMonth: el mes de referencia es aritmética de meses, sin zona
// horaria ni hora del día (decisión 4).
func TestReferenceMonth(t *testing.T) {
	t.Run("enero con offset 2 → noviembre anterior", func(t *testing.T) {
		got := ReferenceMonth(time.Date(2020, 1, 15, 10, 30, 0, 0, time.UTC), 2)
		want := time.Date(2019, 11, 1, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("ReferenceMonth = %s, se esperaba %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
		}
	})
	t.Run("cambio de año", func(t *testing.T) {
		got := ReferenceMonth(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC), 1)
		want := time.Date(2020, 12, 1, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("ReferenceMonth = %s, se esperaba %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
		}
	})
	t.Run("offset 0 normaliza a día 1", func(t *testing.T) {
		got := ReferenceMonth(time.Date(2020, 3, 17, 23, 59, 59, 0, time.UTC), 0)
		want := time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("ReferenceMonth = %s, se esperaba %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
		}
	})
	t.Run("normaliza zona horaria y día", func(t *testing.T) {
		madrid := time.FixedZone("CET", 3600)
		in := time.Date(2020, 1, 1, 23, 30, 0, 0, madrid)
		got := ReferenceMonth(in, 2)
		want := time.Date(2019, 11, 1, 0, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("ReferenceMonth = %s, se esperaba %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
		}
	})
}
