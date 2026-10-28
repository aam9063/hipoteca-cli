package cli

import (
	"encoding/json"
	"fmt"

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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := versionOutput{
				SchemaVersion: 1,
				Version:       info.Version,
				Commit:        info.Commit,
				Date:          info.Date,
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetEscapeHTML(false)
			if err := enc.Encode(out); err != nil {
				return fmt.Errorf("escribiendo el resultado: %w", err)
			}
			return nil
		},
	}
}
