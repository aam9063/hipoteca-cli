// Package cli owns the cobra command tree: argument parsing, flags and the shape of
// what reaches stdout and stderr. It performs no calculation, no network access and no
// file access; those belong to internal/app and its dependencies. The orchestration
// dependencies are injected as Deps so tests can run the whole tree without red.
package cli

import (
	"fmt"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/spf13/cobra"
)

// BuildInfo carries the values injected at release time.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Deps son las dependencias que los comandos resuelven al ejecutarse, no al
// construir el árbol: un fallo de configuración es un error del contrato del
// comando, no un panic al arrancar. El campo nil usa las de producción
// (app.NewRuntime); los tests inyectan una función propia para no tocar la red.
// Version es la versión del binario que la producción pasa a app.NewRuntime
// para el User-Agent ante el Banco; NewRootCmdWithDeps la rellena desde
// BuildInfo cuando llega vacía. ConfigPath es la ruta de la config local para
// los comandos de préstamos (LOAN-01) y DataPath la de la base de estado de
// las vigilancias (WATCH-01): la producción resuelve config.ConfigPath y
// config.DataPath y los tests inyectan temporales, de modo que ningún test
// lee ni escribe datos del usuario.
type Deps struct {
	Runtime    func() (*app.Runtime, error)
	ConfigPath func() (string, error)
	DataPath   func() (string, error)
	Version    string
}

// newRuntime es la construcción de producción del runtime; es una variable
// para que los tests del cableado capturen la versión que llega a
// app.NewRuntime sin tocar configuración ni red.
var newRuntime = app.NewRuntime

// runtime resuelve el runtime para la petición en vuelo.
func (d Deps) runtime() (*app.Runtime, error) {
	if d.Runtime != nil {
		return d.Runtime()
	}
	return newRuntime(d.Version)
}

// configPath resuelve la ruta de la config local (sólo resolución de ruta,
// sin acceso a ficheros: leerla y escribirla es de internal/app).
func (d Deps) configPath() (string, error) {
	if d.ConfigPath != nil {
		return d.ConfigPath()
	}
	return config.ConfigPath()
}

// dataPath resuelve la ruta de la base de estado de las vigilancias (WATCH-01:
// config.DataPath, sustituible en los tests).
func (d Deps) dataPath() (string, error) {
	if d.DataPath != nil {
		return d.DataPath()
	}
	return config.DataPath()
}

// NewRootCmd builds the command tree with the production dependencies. It returns a
// command so tests can execute it without touching os.Args or the real stdout.
func NewRootCmd(info BuildInfo) *cobra.Command {
	return NewRootCmdWithDeps(info, Deps{})
}

// NewRootCmdWithDeps builds the command tree over the given dependencies.
func NewRootCmdWithDeps(info BuildInfo, deps Deps) *cobra.Command {
	// La versión inyectada llega al runtime (y de ahí al User-Agent del
	// cliente del Banco) salvo que las dependencias traigan una explícita.
	if deps.Version == "" {
		deps.Version = info.Version
	}
	root := &cobra.Command{
		Use:   "hipoteca",
		Short: "Índices oficiales del Banco de España y cálculos de tu hipoteca",
		Long: `hipoteca lee los tipos de referencia oficiales del mercado hipotecario que publica
el Banco de España (Euríbor, IRPH y otros) y calcula revisiones de cuota, cuadros de
amortización, amortizaciones anticipadas y escenarios de una hipoteca.

La salida es JSON para que la use un agente. No es asesoramiento financiero: calcula y
compara, no recomienda. No está afiliada ni respaldada por el Banco de España.`,
		// Errors and usage are printed by main, not by cobra, so stdout stays clean.
		SilenceUsage:  true,
		SilenceErrors: true,
		// The completion command is not part of the CLI contract (P-01): it is not
		// enabled.
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		// cobra's default would turn an unknown first word into a plain error;
		// classified as usage it reaches main with the stable code 2 (§6.4).
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("subcomando desconocido %q: ejecute \"hipoteca --help\" para ver los disponibles", args[0]), "")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			// P-01: stdout carries a single JSON value per command. With no subcommand
			// there is no result, so the only output is the usage error that main turns
			// into the JSON envelope on stderr (review finding 5): printing the help
			// here would concatenate prose and JSON on stderr and break parsers. The
			// hint tells the human where the help lives. Explicit help stays prose on
			// stdout with exit 0 (decision 7, see output/doc.go).
			return errs.New(errs.KindUsage,
				"esperando un subcomando: no hay ningún resultado que mostrar",
				"ejecute \"hipoteca --help\" para ver los subcomandos disponibles")
		},
	}

	root.AddCommand(newVersionCmd(info), newIndexCmd(deps), newCalcCmd(), newAmortizeCmd(deps), newScenarioCmd(deps), newLoanCmd(deps), newRevisionCmd(deps), newScheduleCmd(deps), newCompareCmd(deps), newWatchCmd(deps))

	// Flag parse errors (unknown flag, bad value) are also usage errors of the
	// contract; the root func propagates to every subcommand.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return errs.New(errs.KindUsage, fmt.Sprintf("bandera o argumento inválido: %v", err), "")
	})

	return root
}
