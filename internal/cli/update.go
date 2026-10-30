package cli

import (
	"github.com/aam9063/hipoteca-cli/internal/output"
	"github.com/aam9063/hipoteca-cli/internal/update"
	"github.com/spf13/cobra"
)

// newUpdateCmd: `hipoteca update [--check]` (REL-01, S5/S6). Sin --check
// descarga la última release, VERIFICA su checksum contra el checksums.txt
// publicado y reemplaza este binario con escritura atómica: un fallo de red,
// un checksum que no cuadra o una descarga corrupta dejan la instalación
// anterior intacta (S5). Con --check no descarga nada y responde en JSON con
// la versión actual, la disponible y si hay actualización (S6).
//
// Los tests inyectan el servidor y el destino a través de Deps (S8): ninguna
// prueba de este comando toca la red ni el binario real del host.
func newUpdateCmd(info BuildInfo, deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Comprueba la última release publicada y, si quieres, actualiza este binario",
		Long: `Comprueba en GitHub la última release publicada de hipoteca-cli y responde
con la versión de este binario, la disponible y si hay una más nueva.

Sin --check, además descarga la release, verifica su checksum contra el
checksums.txt publicado y reemplaza este binario con escritura atómica: si la
red falla, el checksum no cuadra o la descarga está corrupta, la instalación
anterior queda intacta y funcionando — nada se escribe hasta que el checksum
cuadra.

update es la única orden de la CLI que sustituye el binario en ejecución. No
toca tu config, tus préstamos ni ningún dato: sólo el binario.`,
		Args: noArgs("update"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			check, _ := cmd.Flags().GetBool("check")
			up, err := deps.updater(info.Version)
			if err != nil {
				return err
			}
			var res update.Result
			if check {
				res, err = up.Check(cmd.Context())
			} else {
				res, err = up.Apply(cmd.Context())
			}
			if err != nil {
				return err
			}
			// P-01: un único valor JSON en stdout, por la única vía de escritura.
			if err := output.WriteJSON(cmd.OutOrStdout(), updateOutput{
				SchemaVersion: 1,
				Result:        res,
			}); err != nil {
				return errWritingResult(err)
			}
			return nil
		},
	}
	cmd.Flags().Bool("check", false,
		"sólo comprobar: no descarga ni escribe nada, responde en JSON")
	return cmd
}

// updateOutput es la salida v1 de `hipoteca update` (P-09): el resultado de
// internal/update con la marca de schema_version. update.Result va embebido y
// sus campos suben al JSON tal cual (como LoanView en loan show).
type updateOutput struct {
	SchemaVersion int `json:"schema_version"`
	update.Result
}

// updater construye el Updater de `hipoteca update`: producción apunta a
// GitHub y al binario en ejecución; los tests sustituyen API, base de
// descarga y destino (S8). Un error aquí ya lleva kind del contrato.
func (d Deps) updater(version string) (*update.Updater, error) {
	up, err := update.New(version)
	if err != nil {
		return nil, err
	}
	if d.UpdateAPIURL != "" {
		up.APIURL = d.UpdateAPIURL
	}
	if d.UpdateBaseURL != "" {
		up.BaseURL = d.UpdateBaseURL
	}
	if d.UpdateDestPath != "" {
		up.DestPath = d.UpdateDestPath
	}
	return up, nil
}
