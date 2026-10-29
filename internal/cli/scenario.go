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
// supuestos del usuario y marcados como assumed (P-06). Sobre un préstamo
// guardado con `<id>` opcional (capital, plazo, diferencial, suelo y
// redondeo del préstamo) o con flags sueltas; nada recomienda (P-08).
func newScenarioCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scenario [id]",
		Short: "Cuota y coste total con valores del índice supuestos por ti",
		Args:  maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			if !flags.Changed("index-values") {
				return errs.New(errs.KindUsage,
					"falta --index-values",
					"ejemplo: hipoteca scenario --index-values 1.5,2,2.5,3 --principal 150000 --months 300 --spread 0.80")
			}
			indexValuesFlag, _ := flags.GetString("index-values")
			values, err := parseIndexValues(indexValuesFlag)
			if err != nil {
				return err
			}

			// Dos caminos: préstamo guardado (con id explícito, o resuelto
			// cuando hay uno y no se han dado las banderas del cálculo
			// suelto) y flags sueltas, el camino de siempre.
			flagsSueltas := flags.Changed("principal") || flags.Changed("months")
			if id != "" || !flagsSueltas {
				if in, ok, err := scenarioDePrestamo(deps, cmd, id, flagsSueltas); ok || err != nil {
					if err != nil {
						return err
					}
					res, err := app.Scenario(*in, values, time.Now())
					if err != nil {
						return err
					}
					return writeResult(cmd, res)
				}
				// ok == false: no hay préstamos guardados; cae al camino de
				// flags con su error de banderas que faltan.
			}
			for _, name := range []string{"principal", "months"} {
				if !flags.Changed(name) {
					return errs.New(errs.KindUsage,
						fmt.Sprintf("falta --%s", name),
						"ejemplo: hipoteca scenario --index-values 1.5,2,2.5,3 --principal 150000 --months 300 --spread 0.80")
				}
			}
			principalValue, _ := flags.GetString("principal")
			months, _ := flags.GetInt("months")
			spreadValue, _ := flags.GetString("spread")
			floorValue, _ := flags.GetString("floor")
			rounding, _ := flags.GetString("rounding")

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
	cmd.Flags().String("principal", "", "capital pendiente en euros, p. ej. 150000.00 (no se admite con un préstamo guardado)")
	cmd.Flags().Int("months", 0, "meses restantes del préstamo sobre los que se calcula la cuota, p. ej. 240 (no se admite con un préstamo guardado)")
	cmd.Flags().String("spread", "0", "diferencial sobre el índice en tanto por ciento, p. ej. 0.80 (con préstamo guardado se usa el de la escritura y la bandera no se admite)")
	cmd.Flags().String("floor", "", "suelo del tipo en tanto por ciento, p. ej. 2.00 (opcional; sin él un índice negativo da un TIN negativo; con préstamo guardado se usa el de la escritura y la bandera no se admite)")
	cmd.Flags().String("rounding", "none", "redondeo de escritura del tipo: none, 1/8, 1/4 o 0.05 (con préstamo guardado se usa el de la escritura y la bandera no se admite)")
	return cmd
}

// scenarioDePrestamo construye las entradas de `scenario` desde un préstamo
// guardado (LOAN-01): capital pendiente y meses restantes actuales, con el
// diferencial, el suelo y el redondeo de la escritura. Con id explícito, las
// banderas del cálculo suelto no se admiten. Devuelve ok=false cuando no
// hay préstamos guardados y el id era opcional.
func scenarioDePrestamo(deps Deps, cmd *cobra.Command, id string, flagsSueltas bool) (*app.ScenarioInputs, bool, error) {
	flags := cmd.Flags()
	if id != "" {
		for _, name := range []string{"principal", "months", "spread", "floor", "rounding"} {
			if flags.Changed(name) {
				return nil, false, errs.New(errs.KindUsage,
					fmt.Sprintf("la bandera --%s no se admite junto a un préstamo guardado: el capital, el plazo, el diferencial, el suelo y el redondeo salen del préstamo", name),
					"usa el comando sin ese id para el cálculo suelto")
			}
		}
	} else if flagsSueltas {
		return nil, false, nil
	}
	path, err := deps.configPath()
	if err != nil {
		return nil, false, err
	}
	point, err := app.LoanPointOf(path, id)
	if err != nil {
		if id == "" && isNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &app.ScenarioInputs{
		Loan:       point.ID,
		Principal:  point.Principal,
		TermMonths: point.RemainingMonths,
		Spread:     point.Spread,
		Floor:      point.Floor,
		Rounding:   point.Rounding,
	}, true, nil
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
