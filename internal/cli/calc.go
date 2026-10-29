package cli

import (
	"fmt"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/spf13/cobra"
)

// newCalcCmd: cálculo suelto del sistema francés, sin préstamo guardado y sin
// red. Sólo cifras (decisión 5 de CMD-01): nada aquí es asesoramiento.
func newCalcCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "calc",
		Short: "Cuota y totales de un cálculo suelto, sin guardar el préstamo",
		Args:  noArgs("calc"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags := cmd.Flags()
			for _, name := range []string{"principal", "rate", "months"} {
				if !flags.Changed(name) {
					return errs.New(errs.KindUsage,
						fmt.Sprintf("falta --%s: indícalos los tres", name),
						"ejemplo: hipoteca calc --principal 150000 --rate 3.10 --months 300")
				}
			}
			principalValue, _ := flags.GetString("principal")
			rateValue, _ := flags.GetString("rate")
			months, _ := flags.GetInt("months")

			principal, err := model.NewDecimal(principalValue)
			if err != nil {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("el capital %q no es un decimal válido", principalValue),
					"usa el importe en euros, p. ej. 150000 o 150000.00")
			}
			rate, err := model.NewDecimal(rateValue)
			if err != nil {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("el tipo %q no es un decimal válido", rateValue),
					"usa el tipo nominal anual en tanto por ciento, p. ej. 3.10")
			}

			res, err := app.Calc(principal, rate, months)
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	cmd.Flags().String("principal", "", "capital del préstamo en euros, p. ej. 150000.00")
	cmd.Flags().String("rate", "", "tipo nominal anual en tanto por ciento, p. ej. 3.10")
	cmd.Flags().Int("months", 0, "plazo en meses, p. ej. 300")
	return cmd
}
