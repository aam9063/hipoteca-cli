package cli

import (
	"fmt"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/spf13/cobra"
)

// newCompareCmd (FIN-04): recalcular la historia de un préstamo guardado (o
// de un cálculo suelto con flags) con otro índice y otro diferencial, usando
// el valor publicado de cada mes de referencia, y comparar los dos bloques.
// Sólo cifras (P-08): la salida compara y no recomienda nada.
func newCompareCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compare [id]",
		Short: "Compara tu préstamo con otro índice y diferencial usando los históricos reales",
		Long: `Recalcula la historia de tu préstamo con otro índice y otro diferencial, mes a mes
con los valores oficiales de cada revisión, y compara los dos bloques: la cuota
de cada revisión, el total pagado, el total de intereses y la diferencia por año.

La comparación es sobre el pasado: cubre hasta la última revisión cuyo mes de
referencia está publicado en los dos índices, y lo declara en coverage. Si el
plazo sigue más allá, la salida lo avisa: el resto del plazo no se proyecta.

La diferencia es siempre alternativa − propia: una diferencia positiva significa
que con el índice alternativo se habría pagado más que con el propio. Con un
préstamo fijo el bloque propio es una recomposición hipotética y va marcado
como assumed: un fijo no tiene revisiones de tipo.

Con un préstamo guardado (su id, o el único guardado), el capital, el plazo, el
tramo inicial, el suelo y el redondeo salen del préstamo: --index y --spread son
el índice alternativo y su diferencial. Sin préstamo guardado, el cálculo es un
préstamo hipotético en tramos anuales con --principal y --months, y los valores
del índice son supuestos tuyos con --index-values (marcados como assumed).

Cada revisión usa el índice de su propio mes de referencia; si un mes del índice
alternativo no está publicado el comando responde index_not_published: no
interpola ni inventa valores. La salida compara, no recomienda.`,
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			indexFlag, _ := flags.GetString("index")
			spreadFlag, _ := flags.GetString("spread")
			fromFlag, _ := flags.GetString("from")
			toFlag, _ := flags.GetString("to")

			// Dos caminos: préstamo guardado (con id explícito, o resuelto
			// cuando hay uno y no se han dado las banderas del cálculo
			// suelto) y flags sueltas, el camino hipotético.
			flagsSueltas := flags.Changed("principal") || flags.Changed("months")
			if id != "" || !flagsSueltas {
				if in, ok, err := compareDePrestamo(deps, cmd, id, flagsSueltas); ok || err != nil {
					if err != nil {
						return err
					}
					rt, err := deps.runtime()
					if err != nil {
						return err
					}
					var values []model.Decimal
					if flags.Changed("index-values") {
						v, _ := flags.GetString("index-values")
						values, err = parseIndexValues(v)
						if err != nil {
							return err
						}
					}
					res, err := app.Compare(cmd.Context(), rt, app.CompareParams{
						ConfigPath:  in,
						LoanID:      id,
						Index:       indexFlag,
						Spread:      spreadFlag,
						From:        fromFlag,
						To:          toFlag,
						IndexValues: values,
						Now:         time.Now(),
					})
					if err != nil {
						return err
					}
					return writeResult(cmd, res)
				}
				// ok == false: no hay préstamos guardados; cae al camino de
				// flags con su error de banderas que faltan.
			}
			// Camino sin préstamo guardado: préstamo hipotético y valores
			// supuestos para los DOS bloques; la ventana no aplica porque no
			// hay calendario.
			if flags.Changed("from") || flags.Changed("to") {
				return errs.New(errs.KindUsage,
					"la ventana --from/--to sólo se admite con un préstamo guardado: sin él no hay calendario que acotar",
					"usa un id: hipoteca compare <id> --index <otro> --spread <dif>")
			}
			for _, name := range []string{"principal", "months", "index-values", "index", "spread"} {
				if !flags.Changed(name) {
					return errs.New(errs.KindUsage,
						fmt.Sprintf("falta --%s", name),
						"ejemplo: hipoteca compare --principal 150000 --months 240 --index-values 2.0,2.2,2.4 --index irph --spread 1.00")
				}
			}
			principalValue, _ := flags.GetString("principal")
			months, _ := flags.GetInt("months")
			valuesFlag, _ := flags.GetString("index-values")
			ownSpreadValue, _ := flags.GetString("own-spread")
			floorValue, _ := flags.GetString("floor")
			rounding, _ := flags.GetString("rounding")

			principal, err := model.NewDecimal(principalValue)
			if err != nil {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("el capital %q no es un decimal válido", principalValue),
					"usa el importe en euros, p. ej. 150000 o 150000.00")
			}
			ownSpread, err := model.NewDecimal(ownSpreadValue)
			if err != nil {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("el diferencial propio %q no es un decimal válido", ownSpreadValue),
					"usa el diferencial en tanto por ciento, p. ej. 0.50")
			}
			altSpread, err := model.NewDecimal(spreadFlag)
			if err != nil {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("el diferencial %q no es un decimal válido", spreadFlag),
					"usa el diferencial en tanto por ciento, p. ej. 1.00")
			}
			values, err := parseIndexValues(valuesFlag)
			if err != nil {
				return err
			}
			in := app.CompareLooseInputs{
				Principal:  principal,
				TermMonths: months,
				OwnSpread:  ownSpread,
				Spread:     altSpread,
				Index:      indexFlag,
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
			// El id del índice alternativo debe existir en el catálogo, como
			// en el camino con préstamo: un id inventado es not_found.
			rt, err := deps.runtime()
			if err != nil {
				return err
			}
			if _, err := rt.Catalog.Lookup(indexFlag); err != nil {
				return err
			}
			res, err := app.CompareLoose(in, values, time.Now())
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	cmd.Flags().String("index", "", "id del índice alternativo con el que comparar, p. ej. irph")
	cmd.Flags().String("spread", "", "diferencial del índice alternativo en tanto por ciento, p. ej. 1.00")
	cmd.Flags().String("from", "", "mes desde el que se muestran y comparan las revisiones, YYYY-MM, p. ej. 2025-01 (el recálculo y los totales empiezan siempre en el inicio del préstamo)")
	cmd.Flags().String("to", "", "mes hasta el que se comparan las revisiones, YYYY-MM, p. ej. 2026-01: acota también el cálculo y los totales, que corren hasta la revisión siguiente a la última comparada; no puede ser posterior al último mes publicado: la comparación sólo cubre meses publicados")
	cmd.Flags().String("index-values", "", "valores del índice supuestos, separados por comas: uno por revisión con préstamo guardado, o uno por periodo de 12 meses sin él, p. ej. 2.0,2.2 (cada uno se marca assumed)")
	cmd.Flags().String("principal", "", "capital en euros, p. ej. 150000.00 (sólo sin préstamo guardado)")
	cmd.Flags().Int("months", 0, "plazo total en meses, p. ej. 240 (sólo sin préstamo guardado)")
	cmd.Flags().String("own-spread", "0", "diferencial del bloque propio en tanto por ciento, p. ej. 0.50 (sólo sin préstamo guardado; con él sale del préstamo)")
	cmd.Flags().String("floor", "", "suelo del tipo en tanto por ciento, p. ej. 2.00 (opcional; sólo sin préstamo guardado)")
	cmd.Flags().String("rounding", "none", "redondeo de escritura del tipo: none, 1/8, 1/4 o 0.05 (sólo sin préstamo guardado; con él sale del préstamo)")
	return cmd
}

// compareDePrestamo valida el camino de `compare` sobre un préstamo guardado
// (LOAN-01): las banderas del cálculo hipotético no se admiten. Devuelve la
// ruta de la config; ok=false cuando no hay préstamos guardados y el id era
// opcional, para que el camino de flags responda.
func compareDePrestamo(deps Deps, cmd *cobra.Command, id string, flagsSueltas bool) (string, bool, error) {
	flags := cmd.Flags()
	if id != "" {
		for _, name := range []string{"principal", "months", "own-spread", "floor", "rounding"} {
			if flags.Changed(name) {
				return "", false, errs.New(errs.KindUsage,
					fmt.Sprintf("la bandera --%s no se admite junto a un préstamo guardado: el capital, el plazo, el suelo y el redondeo salen del préstamo", name),
					"usa el comando sin ese id para el cálculo hipotético")
			}
		}
	} else if flagsSueltas {
		return "", false, nil
	}
	path, err := deps.configPath()
	if err != nil {
		return "", false, err
	}
	if _, err := app.LoanPointOf(path, id); err != nil {
		if id == "" && isNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return path, true, nil
}
