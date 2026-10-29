package cli

import (
	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/loan"
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
	cmd.Flags().String("interest-convention", loan.InterestConventionMonthlyNominal, "convención de interés (sólo se soporta monthly-nominal)")
	cmd.Flags().String("payment-rounding", loan.PaymentRoundingHalfUpCents, "redondeo de la cuota (sólo se soporta half-up-cents)")
	cmd.Flags().String("fee-pct", "", "comisión de amortización anticipada en tanto por ciento del importe, p. ej. 0.50; usa 0 si tu escritura no la tiene")
}

// prestamoDeFlags construye un préstamo con las banderas que llegaron
// cambiadas (add guarda tal cual, aunque queden campos de la escritura sin
// declarar: es dato del usuario y show avisa). El suelo "none" o vacío es
// "sin suelo" explícito.
func prestamoDeFlags(cmd *cobra.Command) (loan.Loan, error) {
	flags := cmd.Flags()
	var l loan.Loan
	if flags.Changed("name") {
		l.Name, _ = flags.GetString("name")
	}
	if flags.Changed("type") {
		l.Type, _ = flags.GetString("type")
	}
	if flags.Changed("start-date") {
		l.StartDate, _ = flags.GetString("start-date")
	}
	if flags.Changed("principal") {
		l.OriginalPrincipal, _ = flags.GetString("principal")
	}
	if flags.Changed("months") {
		l.TermMonths, _ = flags.GetInt("months")
	}
	if flags.Changed("payment-day") {
		l.PaymentDay, _ = flags.GetInt("payment-day")
	}
	if flags.Changed("initial-rate") {
		l.InitialRate, _ = flags.GetString("initial-rate")
	}
	if flags.Changed("initial-period-months") {
		v, _ := flags.GetInt("initial-period-months")
		l.InitialPeriodMonths = &v
	}
	if flags.Changed("index") {
		l.Index, _ = flags.GetString("index")
	}
	if flags.Changed("spread") {
		l.Spread, _ = flags.GetString("spread")
	}
	if flags.Changed("spread-unbonified") {
		l.SpreadUnbonified, _ = flags.GetString("spread-unbonified")
	}
	if flags.Changed("revision-every-months") {
		l.RevisionEveryMonths, _ = flags.GetInt("revision-every-months")
	}
	if flags.Changed("reference-offset-months") {
		v, _ := flags.GetInt("reference-offset-months")
		l.ReferenceOffsetMonths = &v
	}
	if flags.Changed("floor") {
		v, _ := flags.GetString("floor")
		if v == "none" {
			v = ""
		}
		l.FloorRate = &v
	}
	if flags.Changed("rate-rounding") {
		l.RateRounding, _ = flags.GetString("rate-rounding")
	}
	l.InterestConvention, _ = flags.GetString("interest-convention")
	l.PaymentRounding, _ = flags.GetString("payment-rounding")
	if flags.Changed("fee-pct") {
		l.EarlyRepaymentFeePct, _ = flags.GetString("fee-pct")
	}
	// Los valores presentes se validan aquí y en la capa de aplicación: el
	// usuario ve el error antes de que se escriba nada en la config.
	if err := l.ValidateValues(); err != nil {
		return loan.Loan{}, err
	}
	return l, nil
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
			l, err := prestamoDeFlags(cmd)
			if err != nil {
				return err
			}
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			if err := app.LoanAdd(path, args[0], l); err != nil {
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
			// Se validan los valores de las banderas antes de abrir la config.
			cambios, err := prestamoDeFlags(cmd)
			if err != nil {
				return err
			}
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			err = app.LoanEdit(path, args[0], func(l *loan.Loan) {
				if cmd.Flags().Changed("name") {
					l.Name = cambios.Name
				}
				if cmd.Flags().Changed("type") {
					l.Type = cambios.Type
				}
				if cmd.Flags().Changed("start-date") {
					l.StartDate = cambios.StartDate
				}
				if cmd.Flags().Changed("principal") {
					l.OriginalPrincipal = cambios.OriginalPrincipal
				}
				if cmd.Flags().Changed("months") {
					l.TermMonths = cambios.TermMonths
				}
				if cmd.Flags().Changed("payment-day") {
					l.PaymentDay = cambios.PaymentDay
				}
				if cmd.Flags().Changed("initial-rate") {
					l.InitialRate = cambios.InitialRate
				}
				if cmd.Flags().Changed("initial-period-months") {
					l.InitialPeriodMonths = cambios.InitialPeriodMonths
				}
				if cmd.Flags().Changed("index") {
					l.Index = cambios.Index
				}
				if cmd.Flags().Changed("spread") {
					l.Spread = cambios.Spread
				}
				if cmd.Flags().Changed("spread-unbonified") {
					l.SpreadUnbonified = cambios.SpreadUnbonified
				}
				if cmd.Flags().Changed("revision-every-months") {
					l.RevisionEveryMonths = cambios.RevisionEveryMonths
				}
				if cmd.Flags().Changed("reference-offset-months") {
					l.ReferenceOffsetMonths = cambios.ReferenceOffsetMonths
				}
				if cmd.Flags().Changed("floor") {
					l.FloorRate = cambios.FloorRate
				}
				if cmd.Flags().Changed("rate-rounding") {
					l.RateRounding = cambios.RateRounding
				}
				if cmd.Flags().Changed("interest-convention") {
					l.InterestConvention = cambios.InterestConvention
				}
				if cmd.Flags().Changed("payment-rounding") {
					l.PaymentRounding = cambios.PaymentRounding
				}
				if cmd.Flags().Changed("fee-pct") {
					l.EarlyRepaymentFeePct = cambios.EarlyRepaymentFeePct
				}
			})
			if err != nil {
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
	return &cobra.Command{
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
			return writeResult(cmd, res)
		},
	}
}

// newLoanRemoveCmd: `loan remove <id> --yes` — sin --yes responde
// confirmation_required/2 y no toca nada (decisión 3 de LOAN-01).
func newLoanRemoveCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Borra un préstamo guardado (irreversible; pide --yes)",
		Args:  exactArgs("loan remove", 1, "\"hipoteca loan list\" muestra los ids guardados"),
		RunE: func(cmd *cobra.Command, args []string) error {
			yes, _ := cmd.Flags().GetBool("yes")
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			if err := app.LoanRemove(path, args[0], yes); err != nil {
				return err
			}
			return writeResult(cmd, removedResult{SchemaVersion: 1, Loan: args[0], Removed: true})
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
			s := loan.Sync{
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
