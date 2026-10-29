package cli

import (
	"strings"
	"testing"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// Decisión 3 de CMD-01: `revision` y `schedule` sin préstamo guardado
// responden loan_incomplete (código 2) con el hint apuntando a `loan add`,
// porque los préstamos guardados son LOAN-01 y aquí no se inventa ningún
// dato. Es preferible un error honesto a un comando que finge funciona.
func TestE2ERevisionYScheduleSinPrestamo(t *testing.T) {
	deps := depsCon(t, &stubSource{})
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

// Más de un argumento (ningún comando de préstamo acepta varios ids) es uso.
func TestE2ERevisionYScheduleConDemasiadosArgumentos(t *testing.T) {
	deps := depsCon(t, &stubSource{})
	for _, cmd := range []string{"revision", "schedule"} {
		t.Run(cmd, func(t *testing.T) {
			stdout, _, err := runRoot(t, deps, cmd, "casa", "piso")
			assertContractError(t, err, errs.KindUsage, 2)
			assertEmpty(t, "stdout", stdout)
		})
	}
}
