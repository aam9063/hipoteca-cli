package cli

import (
	"fmt"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/spf13/cobra"
)

// newLoanCmd monta el grupo `loan`: add, edit, show, list, remove y sync
// (LOAN-01.2). Los préstamos viven en la config local ([loans.<id>], 0600 y
// escritura atómica de CORE-02); el grupo no toca la red.
func newLoanCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "loan",
		Short: "Préstamos guardados en tu config local: alta, edición, borrado y sincronización",
		// Igual que la raíz: sin subcomando no hay resultado, y el único
		// output es el error de uso que main convierte en sobre (P-01).
		RunE: func(_ *cobra.Command, _ []string) error {
			return errs.New(errs.KindUsage,
				"esperando un subcomando de \"hipoteca loan\": no hay ningún resultado que mostrar",
				"ejecute \"hipoteca loan --help\" para ver los subcomandos disponibles")
		},
	}
	cmd.AddCommand(
		newLoanAddCmd(deps),
		newLoanEditCmd(deps),
		newLoanShowCmd(deps),
		newLoanListCmd(deps),
		newLoanRemoveCmd(deps),
		newLoanSyncCmd(deps),
	)
	return cmd
}

// flagsPrestamo registra las banderas del préstamo en un subcomando. Las
// convenciones de cálculo traen por defecto los únicos valores soportados
// (no son datos de la escritura); los que sí lo son no traen valor por
// defecto: o se declaran o el préstamo queda incompleto.
func flagsPrestamo(cmd *cobra.Command) {
	cmd.Flags().String("name", "", "nombre con el que reconoces el préstamo, p. ej. Casa")
	cmd.Flags().String("type", "", "tipo de préstamo: variable, mixta o fija")
	cmd.Flags().String("start-date", "", "fecha de la primera cuota o de firma, según escritura (YYYY-MM-DD)")
	cmd.Flags().String("principal", "", "capital original en euros, p. ej. 180000.00")
	cmd.Flags().Int("months", 0, "plazo total en meses, p. ej. 360")
	cmd.Flags().Int("payment-day", 0, "día del mes en que se paga la cuota, p. ej. 1")
	cmd.Flags().String("initial-rate", "", "TIN anual % del periodo inicial, p. ej. 1.25")
	cmd.Flags().Int("initial-period-months", 0, "meses a tipo fijo antes de la primera revisión, p. ej. 12")
	cmd.Flags().String("index", "", "id del índice de revisión; \"hipoteca index list\" muestra los disponibles")
	cmd.Flags().String("spread", "", "diferencial sobre el índice en puntos, p. ej. 0.89")
	cmd.Flags().String("spread-unbonified", "", "diferencial sin bonificaciones en puntos, p. ej. 1.39 (opcional)")
	cmd.Flags().Int("revision-every-months", 0, "meses entre revisiones, p. ej. 12")
	cmd.Flags().Int("reference-offset-months", 0, "meses de índice antes del mes de revisión, según escritura, p. ej. 2 (0 es un valor válido)")
	cmd.Flags().String("floor", "", "suelo del tipo en tanto por ciento, p. ej. 2.00; usa none o cadena vacía para sin suelo (dato de la escritura: no se supone)")
	cmd.Flags().String("rate-rounding", "", "redondeo de escritura del tipo: none, 1/8, 1/4 o 0.05")
	cmd.Flags().String("interest-convention", app.InterestConventionMonthlyNominal, "convención de interés (sólo se soporta monthly-nominal)")
	cmd.Flags().String("payment-rounding", app.PaymentRoundingHalfUpCents, "redondeo de la cuota (sólo se soporta half-up-cents)")
	cmd.Flags().String("fee-pct", "", "comisión de amortización anticipada en tanto por ciento del importe, p. ej. 0.50; usa 0 si tu escritura no la tiene")
}

// cambiosDeFlags traduce las banderas que llegaron cambiadas a la entrada de
// app (REF-01: la CLI no construye el préstamo, pide a app). Cada campo
// presente va con puntero; los ausentes no se tocan. add guarda tal cual,
// aunque queden campos de la escritura sin declarar: es dato del usuario y
// show avisa. El suelo "none" o vacío es "sin suelo" explícito. La validación
// de valores ocurre en app: `add` valida el préstamo construido de las
// banderas antes de abrir la config; `edit` fusiona sobre el guardado y
// valida el resultado fusionado, porque las banderas solas no describen el
// préstamo resultante (validarlas solas rechazaría cambiar un solo campo,
// p. ej. el tipo no llega y no es un error).
func cambiosDeFlags(cmd *cobra.Command) app.LoanInput {
	flags := cmd.Flags()
	str := func(name string) *string {
		if flags.Changed(name) {
			v, _ := flags.GetString(name)
			return &v
		}
		return nil
	}
	entero := func(name string) *int {
		if flags.Changed(name) {
			v, _ := flags.GetInt(name)
			return &v
		}
		return nil
	}
	var in app.LoanInput
	in.Name = str("name")
	in.Type = str("type")
	in.StartDate = str("start-date")
	in.OriginalPrincipal = str("principal")
	in.TermMonths = entero("months")
	in.PaymentDay = entero("payment-day")
	in.InitialRate = str("initial-rate")
	in.InitialPeriodMonths = entero("initial-period-months")
	in.Index = str("index")
	in.Spread = str("spread")
	in.SpreadUnbonified = str("spread-unbonified")
	in.RevisionEveryMonths = entero("revision-every-months")
	in.ReferenceOffsetMonths = entero("reference-offset-months")
	if flags.Changed("floor") {
		v, _ := flags.GetString("floor")
		if v == "none" {
			v = ""
		}
		in.FloorRate = &v
	}
	in.RateRounding = str("rate-rounding")
	in.InterestConvention = str("interest-convention")
	in.PaymentRounding = str("payment-rounding")
	in.EarlyRepaymentFeePct = str("fee-pct")
	return in
}

// altaDeFlags es la variante de `add`: el préstamo se guarda entero desde las
// banderas, así que las convenciones de cálculo entran siempre (sus valores
// por defecto son los únicos soportados), lleguen cambiadas o no.
func altaDeFlags(cmd *cobra.Command) app.LoanInput {
	in := cambiosDeFlags(cmd)
	convencion, _ := cmd.Flags().GetString("interest-convention")
	in.InterestConvention = &convencion
	redondeo, _ := cmd.Flags().GetString("payment-rounding")
	in.PaymentRounding = &redondeo
	return in
}

// newLoanAddCmd: `loan add <id>` con las banderas del préstamo. Guarda
// aunque el préstamo quede incompleto (es dato del usuario); los comandos
// que calculan responderán loan_incomplete hasta completarlo.
func newLoanAddCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <id>",
		Short: "Guarda un préstamo nuevo en la config local",
		Args:  exactArgs("loan add", 1, "ejemplo: hipoteca loan add casa --name Casa --type variable --start-date 2021-06-01 --principal 180000 --months 360"),
		RunE: func(cmd *cobra.Command, args []string) error {
			in := altaDeFlags(cmd)
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			if err := app.LoanAdd(path, args[0], in); err != nil {
				return err
			}
			// La salida es el préstamo guardado, tal cual quedó.
			res, err := app.LoanShow(path, args[0])
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	flagsPrestamo(cmd)
	return cmd
}

// newLoanEditCmd: `loan edit <id>` cambia sólo las banderas que llegan.
func newLoanEditCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Cambia los campos indicados de un préstamo guardado",
		Args:  exactArgs("loan edit", 1, "\"hipoteca loan list\" muestra los ids guardados"),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Sólo se traducen las banderas: la validación de valores ocurre
			// sobre el préstamo ya fusionado en app.LoanEdit, que es el
			// resultado real de la edición. Un valor presente inválido (mal
			// enum, mes negativo) sigue fallando con usage/2 nombrando el
			// campo, y no se escribe nada en la config.
			cambios := cambiosDeFlags(cmd)
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			if err := app.LoanEdit(path, args[0], cambios); err != nil {
				return err
			}
			res, err := app.LoanShow(path, args[0])
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	flagsPrestamo(cmd)
	return cmd
}

// newLoanShowCmd: `loan show [<id>]` — el id es opcional con un único
// préstamo guardado (LOAN-01.4). Un préstamo incompleto se muestra con sus
// campos ausentes y un aviso por cada uno.
func newLoanShowCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "show [id]",
		Short: "Muestra un préstamo guardado, con los campos de la escritura que falten",
		Args:  maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			res, err := app.LoanShow(path, id)
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
}

// newLoanListCmd: `loan list`.
func newLoanListCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Lista los préstamos guardados con su marca de completo",
		Args:  noArgs("loan list"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			res, err := app.LoanList(path)
			if err != nil {
				return err
			}
			// --pretty es la vista para humanos (PRETTY-01, S1); el JSON
			// sigue siendo el defecto: es el contrato de los agentes.
			if pretty, _ := cmd.Flags().GetBool("pretty"); pretty {
				_, err := fmt.Fprint(cmd.OutOrStdout(), renderLoanListTable(res))
				return err
			}
			return writeResult(cmd, res)
		},
	}
	cmd.Flags().Bool("pretty", false, "tabla legible para humanos: id, nombre, tipo y marca de completo de cada préstamo")
	return cmd
}

// newLoanRemoveCmd: `loan remove [<id>] --yes` — sin --yes responde
// confirmation_required/2 y no toca nada (decisión 3 de LOAN-01). El id es
// opcional con un único préstamo guardado, igual que show, sync, revision y
// schedule (LOAN-01.4).
func newLoanRemoveCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [id]",
		Short: "Borra un préstamo guardado (irreversible; pide --yes)",
		Args:  maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			yes, _ := cmd.Flags().GetBool("yes")
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			resolved, err := app.LoanRemove(path, id, yes)
			if err != nil {
				return err
			}
			return writeResult(cmd, removedResult{SchemaVersion: 1, Loan: resolved, Removed: true})
		},
	}
	cmd.Flags().Bool("yes", false, "confirma el borrado: sin ella no se borra nada")
	return cmd
}

// removedResult es la salida de `loan remove`.
type removedResult struct {
	SchemaVersion int    `json:"schema_version"`
	Loan          string `json:"loan"`
	Removed       bool   `json:"removed"`
}

// newLoanSyncCmd: `loan sync [<id>] --date --outstanding --rate` — guarda el
// recibo real (decisión 2 de LOAN-01: a partir de aquí manda el recibo).
func newLoanSyncCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync [id]",
		Short: "Sincroniza el préstamo con tu último recibo: capital pendiente y tipo real",
		Args:  maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			dateValue, _ := flags.GetString("date")
			outstandingValue, _ := flags.GetString("outstanding")
			rateValue, _ := flags.GetString("rate")
			s := app.LoanSyncInput{
				Date:                 dateValue,
				OutstandingPrincipal: outstandingValue,
				CurrentRate:          rateValue,
			}
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			if err := app.LoanSync(path, id, s); err != nil {
				return err
			}
			res, err := app.LoanShow(path, id)
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	cmd.Flags().String("date", "", "fecha del recibo (YYYY-MM-DD), p. ej. 2026-10-01")
	cmd.Flags().String("outstanding", "", "capital pendiente según el recibo en euros, p. ej. 152341.18")
	cmd.Flags().String("rate", "", "TIN vigente según el recibo en tanto por ciento, p. ej. 3.12")
	return cmd
}
