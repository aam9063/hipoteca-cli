package cli

import (
	"fmt"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/spf13/cobra"
)

// newAmortizeCmd: amortización anticipada sobre el cálculo suelto, sin
// préstamo guardado y sin red. Sólo cifras (P-08): en modo compare devuelve
// las dos opciones y no recomienda ninguna.
func newAmortizeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "amortize",
		Short: "Efecto de una amortización anticipada: cuota menor, menos plazo o ambas",
		Long: `Calcula el efecto de amortizar anticipadamente sobre un cálculo suelto, sin
préstamo guardado y sin red. En modo compare devuelve las dos opciones con sus
cifras, sin recomendar ninguna.

Cuando el importe agota el saldo, el préstamo cierra por el propio evento y la
salida lo marca con closes_at_event (el mes del cierre). En ese caso
payment_after significa cosas distintas según el modo: en reduce-payment es
0.00, porque no queda plazo sobre el que recalcular una cuota; en reduce-term
es la cuota mantenida, que es la del propio mes del evento y sí se cobró. En
ambos el cálculo termina en ese mes: después del evento no existe ninguna
cuota posterior.`,
		Args: noArgs("amortize"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags := cmd.Flags()
			for _, name := range []string{"principal", "rate", "months", "amount", "date", "mode"} {
				if !flags.Changed(name) {
					return errs.New(errs.KindUsage,
						fmt.Sprintf("falta --%s: indica las seis", name),
						"ejemplo: hipoteca amortize --principal 150000 --rate 3.10 --months 300 --amount 10000 --date 60 --mode compare")
				}
			}
			principalValue, _ := flags.GetString("principal")
			rateValue, _ := flags.GetString("rate")
			months, _ := flags.GetInt("months")
			amountValue, _ := flags.GetString("amount")
			feePctValue, _ := flags.GetString("fee-pct")
			afterMonth, _ := flags.GetInt("date")
			mode, _ := flags.GetString("mode")

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
			amount, err := model.NewDecimal(amountValue)
			if err != nil {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("el importe de la amortización %q no es un decimal válido", amountValue),
					"usa el importe en euros, p. ej. 10000 o 10000.00")
			}
			// FIN-03: la comisión se paga aparte y no reduce el capital. Sin
			// bandera (o con 0) no hay comisión.
			feePct, err := model.NewDecimal(feePctValue)
			if err != nil {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("la comisión %q no es un decimal válido", feePctValue),
					"usa la comisión en tanto por ciento del importe, p. ej. 0.50")
			}

			// La atribución lleva la fecha de elaboración: el día de la
			// ejecución, en UTC (Amortize no consulta ningún dato del Banco).
			res, err := app.Amortize(app.AmortizeInputs{
				Principal:     principal,
				AnnualRatePct: rate,
				TermMonths:    months,
				Amount:        amount,
				AfterMonth:    afterMonth,
				Mode:          mode,
				FeePct:        feePct,
			}, time.Now())
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	cmd.Flags().String("principal", "", "capital del préstamo en euros, p. ej. 150000.00")
	cmd.Flags().String("rate", "", "tipo nominal anual en tanto por ciento, p. ej. 3.10")
	cmd.Flags().Int("months", 0, "plazo total en meses, p. ej. 300")
	cmd.Flags().String("amount", "", "importe a amortizar anticipadamente en euros, p. ej. 10000.00")
	cmd.Flags().String("fee-pct", "0", "comisión por la amortización anticipada en tanto por ciento del importe, p. ej. 0.50 (por defecto 0; se paga aparte y no reduce el capital)")
	// Decisión 3 de CMD-02: --date es el mes del evento (índice de cuota desde
	// el inicio del cálculo), no una fecha del calendario: sin préstamo
	// guardado no hay calendario.
	cmd.Flags().Int("date", 0, "mes del evento: número de la cuota (1, 2, 3…) tras la que se amortiza, contado desde el inicio del cálculo; no es una fecha de calendario, p. ej. 60")
	cmd.Flags().String("mode", "", "modo de la amortización: reduce-payment, reduce-term o compare (devuelve las dos opciones, sin recomendar)")
	return cmd
}
