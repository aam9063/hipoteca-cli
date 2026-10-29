package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/spf13/cobra"
)

// newScenarioCmd: cuota y coste total para distintos valores del índice,
// supuestos del usuario y marcados como assumed (P-06). Sin red y sin
// préstamo guardado; nada recomienda (P-08).
func newScenarioCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scenario",
		Short: "Cuota y coste total con valores del índice supuestos por ti",
		Args:  noArgs("scenario"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags := cmd.Flags()
			for _, name := range []string{"index-values", "principal", "months"} {
				if !flags.Changed(name) {
					return errs.New(errs.KindUsage,
						fmt.Sprintf("falta --%s", name),
						"ejemplo: hipoteca scenario --index-values 1.5,2,2.5,3 --principal 150000 --months 300 --spread 0.80")
				}
			}
			indexValuesFlag, _ := flags.GetString("index-values")
			principalValue, _ := flags.GetString("principal")
			months, _ := flags.GetInt("months")
			spreadValue, _ := flags.GetString("spread")
			floorValue, _ := flags.GetString("floor")
			rounding, _ := flags.GetString("rounding")

			values, err := parseIndexValues(indexValuesFlag)
			if err != nil {
				return err
			}
			principal, err := model.NewDecimal(principalValue)
			if err != nil {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("el capital %q no es un decimal válido", principalValue),
					"usa el importe en euros, p. ej. 150000 o 150000.00")
			}
			spread, err := model.NewDecimal(spreadValue)
			if err != nil {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("el diferencial %q no es un decimal válido", spreadValue),
					"usa el diferencial en tanto por ciento, p. ej. 0.80")
			}
			in := app.ScenarioInputs{
				Principal:  principal,
				TermMonths: months,
				Spread:     spread,
				Rounding:   rounding,
			}
			if flags.Changed("floor") {
				floor, err := model.NewDecimal(floorValue)
				if err != nil {
					return errs.New(errs.KindUsage,
						fmt.Sprintf("el suelo %q no es un decimal válido", floorValue),
						"usa el suelo en tanto por ciento, p. ej. 2.00")
				}
				in.Floor = &floor
			}

			// La atribución lleva la fecha de elaboración: el día de la
			// ejecución, en UTC (Scenario no consulta ningún dato del Banco).
			res, err := app.Scenario(in, values, time.Now())
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	cmd.Flags().String("index-values", "", "valores del índice supuestos, separados por comas, p. ej. 1.50,2,2.5,3 (cada uno se devuelve tal cual y se marca assumed)")
	cmd.Flags().String("principal", "", "capital pendiente en euros, p. ej. 150000.00")
	cmd.Flags().Int("months", 0, "meses restantes del préstamo sobre los que se calcula la cuota, p. ej. 240")
	cmd.Flags().String("spread", "0", "diferencial sobre el índice en tanto por ciento, p. ej. 0.80")
	cmd.Flags().String("floor", "", "suelo del tipo en tanto por ciento, p. ej. 2.00 (opcional; sin él un índice negativo da un TIN negativo)")
	cmd.Flags().String("rounding", "none", "redondeo de escritura del tipo: none, 1/8, 1/4 o 0.05")
	return cmd
}

// parseIndexValues parte --index-values por comas y parsea cada valor: se
// conservan tal cual (con su escala) para el eco de la salida.
func parseIndexValues(value string) ([]model.Decimal, error) {
	parts := strings.Split(value, ",")
	values := make([]model.Decimal, 0, len(parts))
	for _, part := range parts {
		s := strings.TrimSpace(part)
		v, err := model.NewDecimal(s)
		if err != nil {
			return nil, errs.New(errs.KindUsage,
				fmt.Sprintf("el valor de índice %q no es un decimal válido", s),
				"pasa los valores separados por comas, p. ej. --index-values 1.5,2,2.5,3")
		}
		values = append(values, v)
	}
	return values, nil
}
