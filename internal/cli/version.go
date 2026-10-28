package cli

import (
	"fmt"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/output"
	"github.com/spf13/cobra"
)

// versionOutput is the v1 contract of `hipoteca version` (P-09): fields are additive
// only, and the shape follows P-01 (one JSON value on stdout).
type versionOutput struct {
	SchemaVersion int    `json:"schema_version"`
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	Date          string `json:"date"`
}

func newVersionCmd(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Muestra la versión, el commit y la fecha de compilación",
		// Review finding 4: cobra.NoArgs would error before the root wrappers, so
		// the process would exit 1 (internal) instead of 2 (usage). An Args function
		// returning an errs error keeps the exit code in the contract table (§6.4).
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return errs.New(errs.KindUsage,
					fmt.Sprintf("\"hipoteca version\" no acepta argumentos: se ha recibido %q", args[0]),
					"ejecute \"hipoteca version --help\" para ver su uso")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := versionOutput{
				SchemaVersion: 1,
				Version:       info.Version,
				Commit:        info.Commit,
				Date:          info.Date,
			}
			// Review finding 6: output.WriteJSON is the one writer of the CLI; the
			// version result does not bypass it with its own encoder.
			if err := output.WriteJSON(cmd.OutOrStdout(), out); err != nil {
				return fmt.Errorf("escribiendo el resultado: %w", err)
			}
			return nil
		},
	}
}
