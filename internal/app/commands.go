package app

import (
	"fmt"

	"github.com/aam9063/hipoteca-cli/internal/errs"
)

// ErrLoanIncomplete es la respuesta honesta de `revision` y `schedule` cuando
// no hay un préstamo guardado (decisión 3 de CMD-01): los préstamos guardados
// existen desde LOAN-01 y hasta que haya uno no se inventa ningún dato de la
// escritura.
func ErrLoanIncomplete(command string) error {
	return errs.New(errs.KindLoanIncomplete,
		fmt.Sprintf("\"hipoteca %s\" necesita un préstamo guardado y no hay ninguno", command),
		"guarda primero el préstamo con \"hipoteca loan add\"")
}
