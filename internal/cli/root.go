// Package cli owns the cobra command tree: argument parsing, flags and the shape of
// what reaches stdout and stderr. It performs no calculation, no network access and no
// file access; those belong to internal/app and its dependencies.
package cli

import (
	"fmt"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/spf13/cobra"
)

// BuildInfo carries the values injected at release time.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// NewRootCmd builds the command tree. It returns a command so tests can execute it
// without touching os.Args or the real stdout.
func NewRootCmd(info BuildInfo) *cobra.Command {
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			// P-01: stdout carries a single JSON value per command. With no subcommand
			// there is no result, so the help goes to stderr and the command fails
			// with a usage error (exit code 2), leaving stdout empty.
			cmd.SetOut(cmd.ErrOrStderr())
			if err := cmd.Help(); err != nil {
				return err
			}
			return errs.New(errs.KindUsage, "esperando un subcomando: ejecute \"hipoteca --help\" para ver los disponibles", "")
		},
	}

	root.AddCommand(newVersionCmd(info))

	// Flag parse errors (unknown flag, bad value) are also usage errors of the
	// contract; the root func propagates to every subcommand.
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return errs.New(errs.KindUsage, fmt.Sprintf("bandera o argumento inválido: %v", err), "")
	})

	return root
}
