package cli

import (
	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/spf13/cobra"
)

// newScheduleCmd: el cuadro de amortización de un préstamo guardado
// (LOAN-01.6). El id es opcional con un único préstamo; sin préstamos
// guardados mantiene la respuesta honesta del stub de CMD-01
// (loan_incomplete/2 con el hint de `loan add`). No toca la red: sólo la
// config local.
func newScheduleCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule [id]",
		Short: "Cuadro de amortización de un préstamo guardado",
		Long: `Construye el cuadro de amortización del préstamo guardado. Con un recibo
sincronizado (loan sync) parte del capital y del tipo reales del recibo; sin él,
del capital original y del tipo inicial — en un préstamo variable eso sólo
describe el tramo inicial, y la salida lo avisa.

--from y --to recortan las filas mostradas por mes (ambos incluidos); los totales
son siempre los del cuadro completo.`,
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			from, _ := flags.GetString("from")
			to, _ := flags.GetString("to")
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			res, err := app.Schedule(app.ScheduleParams{
				ConfigPath: path,
				LoanID:     id,
				From:       from,
				To:         to,
			})
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	cmd.Flags().String("from", "", "primer mes del tramo a mostrar (YYYY-MM o YYYY-MM-DD), p. ej. 2027-01")
	cmd.Flags().String("to", "", "último mes del tramo a mostrar (YYYY-MM o YYYY-MM-DD), p. ej. 2027-12")
	return cmd
}
