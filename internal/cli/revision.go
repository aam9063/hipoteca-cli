package cli

import (
	"fmt"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/spf13/cobra"
)

// newRevisionCmd: la revisión de un préstamo guardado (LOAN-01.5). El id es
// opcional con un único préstamo; sin préstamos guardados mantiene la
// respuesta honesta del stub de CMD-01 (loan_incomplete/2 con el hint de
// `loan add`).
func newRevisionCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revision [id]",
		Short: "Revisión de la cuota de un préstamo guardado: índice, TIN nuevo y cuota nueva",
		Long: `Calcula la próxima revisión de la cuota (o la de una fecha con --date): mes de
referencia según la escritura, valor del índice publicado por el Banco de España,
TIN nuevo con suelo y redondeo de escritura, y cuota nueva con el capital pendiente
y los meses restantes.

El valor del índice sale de la serie oficial; si el mes de referencia aún no está
publicado el comando responde index_not_published. Con --assume se calcula un
escenario con un valor supuesto, marcado como assumed y avisado en la salida:
no es un dato oficial.`,
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			date, _ := flags.GetString("date")
			var assume *model.Decimal
			if flags.Changed("assume") {
				value, _ := flags.GetString("assume")
				v, err := model.NewDecimal(value)
				if err != nil {
					return errs.New(errs.KindUsage,
						fmt.Sprintf("el valor de índice %q no es un decimal válido", value),
						"usa el valor en tanto por ciento, p. ej. --assume 2.35")
				}
				assume = &v
			}
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			rt, err := deps.runtime()
			if err != nil {
				return err
			}
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			res, err := app.Revision(cmd.Context(), rt, app.RevisionParams{
				ConfigPath: path,
				LoanID:     id,
				Date:       date,
				Assume:     assume,
				Now:        time.Now(),
			})
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	cmd.Flags().String("date", "", "mes o fecha de la revisión (YYYY-MM o YYYY-MM-DD); por defecto, la próxima revisión posterior al mes en curso, p. ej. 2027-01")
	cmd.Flags().String("assume", "", "valor del índice supuesto en tanto por ciento, p. ej. 2.35: calcula un escenario marcado como assumed en lugar de leer el valor publicado")
	return cmd
}
