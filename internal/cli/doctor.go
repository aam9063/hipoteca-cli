package cli

import (
	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/config"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/output"
	"github.com/spf13/cobra"
)

// newDoctorCmd: `hipoteca doctor` (AGENT-01, S5/S7). Un diagnóstico completo
// en un único valor JSON: versión y commit, rutas de config y datos con su
// permiso, catálogo de índices, préstamos guardados con los campos que les
// faltan, y UNA comprobación de red que --offline desactiva. No necesita
// runtime: el diagnóstico sobrevive a una config corrupta avisando, así que
// monta él mismo el catálogo desde la ruta de config inyectada.
func newDoctorCmd(info BuildInfo, deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "El estado de la instalación: rutas, permisos, catálogo, préstamos y red",
		Long: `Responde el estado completo de la instalación: versión y commit del binario,
las rutas de config, datos y caché con su permiso, los índices del catálogo
con su código de serie, los préstamos guardados —y, si alguno está
incompleto, el campo de la escritura que le falta— y una única comprobación
de conectividad con el Banco de España.

Con --offline no se toca la red: la salida lo declara con network.checked
false. Doctor es un diagnóstico: informa de lo que encuentra y sale 0.`,
		Args: noArgs("doctor"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			offline, _ := cmd.Flags().GetBool("offline")
			cfgPath, err := deps.configPath()
			if err != nil {
				return err
			}
			dataPath, err := deps.dataPath()
			if err != nil {
				return err
			}
			cacheDir, err := deps.cacheDir()
			if err != nil {
				return err
			}
			res, err := app.Doctor(cmd.Context(), app.DoctorParams{
				ConfigPath: cfgPath,
				DataPath:   dataPath,
				CacheDir:   cacheDir,
				Version:    info.Version,
				Commit:     info.Commit,
				Offline:    offline,
			})
			if err != nil {
				return err
			}
			if err := output.WriteJSON(cmd.OutOrStdout(), res); err != nil {
				return errWritingResult(err)
			}
			return nil
		},
	}
	cmd.Flags().Bool("offline", false,
		"no comprobar la red: la salida declara network.checked false y nada toca al Banco de España")
	return cmd
}

// Deps.cacheDir resuelve el directorio de la caché de series (sólo
// resolución de ruta, sin acceso a ficheros), sustituible en los tests igual
// que ConfigPath y DataPath.
func (d Deps) cacheDir() (string, error) {
	if d.CacheDir != nil {
		return d.CacheDir()
	}
	return config.CacheDir()
}

// errWritingResult envuelve un fallo de escritura del resultado; es un error
// interno inesperado, no un caso del contrato.
func errWritingResult(err error) error {
	return errs.New(errs.KindInternal, "escribiendo el resultado: "+err.Error(), "")
}
