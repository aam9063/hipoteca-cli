package cli

import (
	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/spf13/cobra"
)

// Los préstamos guardados son LOAN-01 y aún no existen: `revision` y
// `schedule` se declaran ya en el árbol pero responden loan_incomplete (código
// 2) con el hint de `loan add` (decisión 3 de CMD-01). Un error honesto antes
// que un comando que finge funciona: aquí no se inventa ningún dato de la
// escritura (mes de referencia, redondeo, suelo, comisión).
func newRevisionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revision [id]",
		Short: "Revisión de la cuota de un préstamo guardado",
		Args:  maxArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.ErrLoanIncomplete("revision")
		},
	}
}

func newScheduleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "schedule [id]",
		Short: "Cuadro de amortización de un préstamo guardado",
		Args:  maxArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return app.ErrLoanIncomplete("schedule")
		},
	}
}
