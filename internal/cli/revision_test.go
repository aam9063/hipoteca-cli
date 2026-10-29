package cli

import (
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// TestE2ERevisionYScheduleSinPrestamo: sin ningún préstamo guardado,
// `revision` y `schedule` mantienen la respuesta honesta del stub de CMD-01
// (loan_incomplete, código 2) con el hint apuntando a `loan add`. La config
// es una temporal inexistente: nada toca la config del usuario.
func TestE2ERevisionYScheduleSinPrestamo(t *testing.T) {
	deps := depsConConfig(t, &stubSource{}, nil)
	for _, cmd := range []string{"revision", "schedule"} {
		t.Run(cmd, func(t *testing.T) {
			stdout, _, err := runRoot(t, deps, cmd)
			e := assertContractError(t, err, errs.KindLoanIncomplete, 2)
			assertEmpty(t, "stdout", stdout)
			if !strings.Contains(e.Hint, "loan add") {
				t.Errorf("el hint debe apuntar a \"loan add\", got %q", e.Hint)
			}
		})
	}
}

// TestE2ERevisionConPrestamoIncompleto: un préstamo sin un dato de la
// escritura responde loan_incomplete/2 nombrando el campo (decisión 1 de
// LOAN-01); no se supone nada.
func TestE2ERevisionConPrestamoIncompleto(t *testing.T) {
	deps := depsConConfig(t, &stubSource{}, nil)
	// Préstamo sin floor_rate ni reference_offset_months: dos datos de la
	// escritura ausentes. `loan add` lo guarda igual: es dato del usuario.
	_, _, err := runRoot(t, deps, "loan", "add", "casa",
		"--name", "Casa", "--type", "variable", "--start-date", "2021-06-01",
		"--principal", "180000", "--months", "360", "--payment-day", "1",
		"--initial-rate", "1.25", "--initial-period-months", "12",
		"--index", "euribor-1y", "--spread", "0.89",
		"--revision-every-months", "12", "--rate-rounding", "none", "--fee-pct", "0")
	if err != nil {
		t.Fatalf("loan add de un préstamo a medias debe guardarse: %v", err)
	}

	stdout, _, err := runRoot(t, deps, "revision", "casa")
	e := assertContractError(t, err, errs.KindLoanIncomplete, 2)
	assertEmpty(t, "stdout", stdout)
	if !strings.Contains(e.Message, "floor_rate") && !strings.Contains(e.Message, "reference_offset_months") {
		t.Errorf("el error debe nombrar el campo que falta: %q", e.Message)
	}
	if !strings.Contains(e.Hint, "loan edit") {
		t.Errorf("el hint debe apuntar a \"loan edit\": %q", e.Hint)
	}
}

// Más de un argumento (ningún comando de préstamo acepta varios ids) es uso.
func TestE2ERevisionYScheduleConDemasiadosArgumentos(t *testing.T) {
	deps := depsConConfig(t, &stubSource{}, nil)
	for _, cmd := range []string{"revision", "schedule"} {
		t.Run(cmd, func(t *testing.T) {
			stdout, _, err := runRoot(t, deps, cmd, "casa", "piso")
			assertContractError(t, err, errs.KindUsage, 2)
			assertEmpty(t, "stdout", stdout)
		})
	}
}
