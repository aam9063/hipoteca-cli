package cli

import (
	"fmt"
	"strconv"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/output"
	"github.com/aam9063/hipoteca-cli/internal/watch"
	"github.com/spf13/cobra"
)

// newWatchCmd monta el grupo `watch`: add, list, remove, check y run
// (WATCH-01). El estado vive en la base local (config.DataPath, 0600); los
// sinks en la config TOML. `check` y `run` son los únicos que tocan la red
// (el Banco y el sink configurado).
func newWatchCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Aviso de nueva publicación de un índice que sigues",
		// Igual que la raíz: sin subcomando no hay resultado, y el único
		// output es el error de uso que main convierte en sobre (P-01).
		RunE: func(_ *cobra.Command, _ []string) error {
			return errs.New(errs.KindUsage,
				"esperando un subcomando de \"hipoteca watch\": no hay ningún resultado que mostrar",
				"ejecute \"hipoteca watch --help\" para ver los subcomandos disponibles")
		},
	}
	cmd.AddCommand(
		newWatchAddCmd(deps),
		newWatchListCmd(deps),
		newWatchRemoveCmd(deps),
		newWatchCheckCmd(deps),
		newWatchRunCmd(deps),
	)
	return cmd
}

// newWatchAddCmd: `watch add index <id> [--sink <nombre>] [--with-impact]`.
// Sin --with-impact el aviso no lleva ningún dato del préstamo (decisión 3);
// con ella, la CLI avisa en stderr de que ese dato saldrá de la máquina (P-03).
func newWatchAddCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add index <id>",
		Short: "Vigila un índice y avisa cuando se publique un mes nuevo",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) != 2 {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("\"hipoteca watch add index\" espera el id del índice y ha recibido %d argumento(s)", len(args)-1),
					"ejemplo: \"hipoteca watch add index euribor-1y\"; \"hipoteca index list\" muestra los disponibles")
			}
			if args[0] != watch.KindIndex {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("el tipo de vigilancia %q no existe; en esta versión sólo hay %q", args[0], watch.KindIndex),
					"ejemplo: \"hipoteca watch add index euribor-1y\"")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			sink, _ := flags.GetString("sink")
			withImpact, _ := flags.GetBool("with-impact")
			rt, err := deps.runtime()
			if err != nil {
				return err
			}
			dbPath, err := deps.dataPath()
			if err != nil {
				return err
			}
			cfgPath, err := deps.configPath()
			if err != nil {
				return err
			}
			res, err := app.WatchAdd(dbPath, cfgPath, rt.Catalog, args[0], args[1], sink, withImpact, time.Now())
			if err != nil {
				return err
			}
			if withImpact {
				// P-03, decisión 3: el usuario debe saber, al configurarlo,
				// que el aviso llevará un dato de su préstamo fuera de la
				// máquina. Diagnóstico en stderr; stdout sigue siendo JSON.
				fmt.Fprintf(cmd.ErrOrStderr(),
					"Aviso (P-03): los avisos de esta vigilancia incluirán el cambio estimado de cuota de tu préstamo y saldrán de esta máquina hacia el sink %q: quien pueda leer el sink puede leer ese dato.\n",
					res.Watch.Sink)
			}
			return writeResult(cmd, res)
		},
	}
	cmd.Flags().String("sink", "webhook", "nombre del sink de la config que recibe los avisos")
	cmd.Flags().Bool("with-impact", false, "incluye en el aviso el cambio estimado de cuota de tu préstamo (P-03: ese dato saldrá de tu máquina hacia el sink)")
	return cmd
}

// newWatchListCmd: `watch list`.
func newWatchListCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Vigilancias guardadas",
		Args:  noArgs("watch list"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			dbPath, err := deps.dataPath()
			if err != nil {
				return err
			}
			res, err := app.WatchList(dbPath)
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
}

// newWatchRemoveCmd: `watch remove <id>`.
func newWatchRemoveCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Borra una vigilancia y su registro de meses vistos",
		Args:  exactArgs("watch remove", 1, "\"hipoteca watch list\" muestra los ids guardados"),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("el id de vigilancia %q no es un número", args[0]),
					"\"hipoteca watch list\" muestra los ids guardados")
			}
			dbPath, err := deps.dataPath()
			if err != nil {
				return err
			}
			res, err := app.WatchRemove(dbPath, id)
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
}

// newWatchCheckCmd: `watch check` — una pasada. Sin dato nuevo responde
// events: [] y termina con 0 (decisión 5); un fallo del sink es error visible
// (código 5) sin borrar el evento grabado.
func newWatchCheckCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Una pasada: avisa de los meses nuevos publicados desde la última",
		Args:  noArgs("watch check"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := deps.runtime()
			if err != nil {
				return err
			}
			dbPath, err := deps.dataPath()
			if err != nil {
				return err
			}
			cfgPath, err := deps.configPath()
			if err != nil {
				return err
			}
			res, err := app.WatchCheck(cmd.Context(), dbPath, cfgPath, rt.Source, rt.Catalog, time.Now())
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
}

// minRunInterval es el intervalo mínimo de `watch run` (arranque §5.3): los
// índices son mensuales; vigilar más a menudo sólo multiplicaría peticiones.
const minRunInterval = time.Hour

// newWatchRunCmd: `watch run [--interval 12h]` — bucle hasta Ctrl+C. El
// intervalo mínimo es 1 h y por defecto 12 h; un valor menor es un error de
// uso, y ésa es también la garantía anti busy-loop.
func newWatchRunCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Bucle de vigilancia: una pasada de check cada intervalo, hasta Ctrl+C",
		Args:  noArgs("watch run"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			flags := cmd.Flags()
			intervalValue, _ := flags.GetString("interval")
			interval, err := time.ParseDuration(intervalValue)
			if err != nil {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("el intervalo %q no es una duración válida", intervalValue),
					"usa una duración de Go, p. ej. 12h o 1h30m; el mínimo es 1h")
			}
			if interval < minRunInterval {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("el intervalo mínimo es 1 hora y has pedido %s: los índices son mensuales", interval),
					"ejemplo: \"hipoteca watch run --interval 12h\"")
			}
			rt, err := deps.runtime()
			if err != nil {
				return err
			}
			dbPath, err := deps.dataPath()
			if err != nil {
				return err
			}
			cfgPath, err := deps.configPath()
			if err != nil {
				return err
			}
			return app.WatchRun(cmd.Context(), app.RunParams{
				DBPath:   dbPath,
				CfgPath:  cfgPath,
				Source:   rt.Source,
				Catalog:  rt.Catalog,
				Interval: interval,
				// Un fallo de pasada no mata el bucle (decisión 5): el evento
				// queda grabado y se reintenta; el sobre sale por stderr.
				Report: func(err error) {
					if env, ok := errs.From(err); ok {
						_ = output.WriteJSON(cmd.ErrOrStderr(), env)
						return
					}
					fmt.Fprintln(cmd.ErrOrStderr(), "Error:", err.Error())
				},
			})
		},
	}
	cmd.Flags().String("interval", "12h", "tiempo entre pasadas; mínimo 1h, por defecto 12h (los índices son mensuales)")
	return cmd
}
