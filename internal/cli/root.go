// Package cli owns the cobra command tree: argument parsing, flags and the shape of
// what reaches stdout and stderr. It performs no calculation, no network access and no
// file access; those belong to internal/app and its dependencies.
package cli

import (
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.AddCommand(newVersionCmd(info))

	return root
}
