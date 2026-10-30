package skills

import (
	"strings"
	"testing"
)

// AGENT-01 S1: `skills list` debe ofrecer las recetas de docs/ARRANQUE.md
// §6.5, cada una con su id y una línea de para qué sirve.
func TestListReturnsTheFourRecipesOfArranque65(t *testing.T) {
	got := List()
	want := []string{"next-revision", "early-repayment", "sensitivity", "index-comparison"}
	if len(got) != len(want) {
		t.Fatalf("List() devolvió %d recetas, want %d (%v)", len(got), len(want), got)
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("List()[%d].ID = %q, want %q", i, got[i].ID, id)
		}
		if got[i].Title == "" {
			t.Errorf("la receta %q no tiene título", id)
		}
		if got[i].Summary == "" || strings.ContainsAny(got[i].Summary, "\n\r") {
			t.Errorf("la receta %q necesita una sola línea de resumen, got %q", id, got[i].Summary)
		}
	}
}

// AGENT-01 S3: cada receta se basta sola: comandos exactos con sus flags,
// y qué hacer ante index_not_published y loan_incomplete.
func TestGetRecipesAreSelfSufficient(t *testing.T) {
	cases := []struct {
		id    string
		exact []string // cadenas que DEBEN aparecer literalmente
	}{
		{
			id: "next-revision",
			exact: []string{
				"hipoteca loan list",
				"hipoteca loan add",
				"hipoteca loan show",
				"hipoteca loan edit",
				"hipoteca revision",
				"--assume",
				"--reference-offset-months",
				"--floor none",
				"index_not_published",
				"loan_incomplete",
				"missing_fields",
				"assumed",
				"Banco de España",
			},
		},
		{
			id: "early-repayment",
			exact: []string{
				"hipoteca amortize",
				"--amount",
				"--mode compare",
				"--fee-pct",
				"--principal",
				"--rate",
				"--months",
				"--date",
				"loan_incomplete",
				"missing_fields",
				"reduce_payment",
				"reduce_term",
				"interest_saved",
				"closes_at_event",
			},
		},
		{
			id: "sensitivity",
			exact: []string{
				"hipoteca scenario",
				"--index-values 1.5,2,2.5,3",
				"--principal",
				"--months",
				"--spread",
				"assumed",
				"loan_incomplete",
				"missing_fields",
			},
		},
		{
			id: "index-comparison",
			exact: []string{
				"hipoteca compare",
				"--index irph",
				"--spread 1.00",
				"--index-values",
				"index_not_published",
				"loan_incomplete",
				"missing_fields",
				"coverage",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			s, ok := Get(tc.id)
			if !ok {
				t.Fatalf("Get(%q) no encontró la receta", tc.id)
			}
			if s.Body == "" {
				t.Fatal("la receta no tiene cuerpo")
			}
			for _, want := range tc.exact {
				if !strings.Contains(s.Body, want) {
					t.Errorf("el cuerpo de %q no menciona %q: un agente sin contexto no podría ejecutarla", tc.id, want)
				}
			}
		})
	}
}

// AGENT-01 S2: los ids de List y Get son el mismo catálogo.
func TestListAndGetShareTheCatalog(t *testing.T) {
	for _, s := range List() {
		if _, ok := Get(s.ID); !ok {
			t.Errorf("Get(%q) no existe pero List lo lista", s.ID)
		}
	}
	if got := IDs(); len(got) != len(List()) {
		t.Errorf("IDs() devolvió %d, want %d", len(got), len(List()))
	}
}

// AGENT-01 S2: un id desconocido no existe; el CLI lo convierte en
// not_found/4 con la lista de ids válidos en el hint.
func TestGetUnknownIDIsMissing(t *testing.T) {
	if _, ok := Get("inventada"); ok {
		t.Fatal("Get(\"inventada\") encontró algo que no debe existir")
	}
}
