package cli

import (
	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/spf13/cobra"
)

// newUninstallCmd: `hipoteca uninstall` (REF-03). Borra los datos de la
// herramienta — la config con los préstamos, la base de datos de las
// vigilancias y la caché de series — tras confirmación explícita con --yes;
// --dry-run lista las rutas y sus tamaños sin borrar nada. El binario no se
// toca: lo retira quien lo instaló (S4).
func newUninstallCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Borra tu config (préstamos incluidos), la base de datos y la caché; el binario no",
		Long: `Borra los datos que hipoteca guarda en esta máquina:

  la configuración (config.toml), con tus préstamos guardados
  la base de datos (hipoteca.db), con las vigilancias y los meses vistos
  la caché de series del Banco de España

Sin --yes no se borra nada: responde confirmation_required y las rutas quedan
intactas. Con --dry-run enumera las rutas y cuánto ocupa cada una, también
sin borrar nada.

El binario NO se borra: lo retira el gestor de paquetes que lo instaló
("scoop uninstall hipoteca" con Scoop, "brew uninstall --cask hipoteca" con
Homebrew) o bórralo tú del directorio donde lo tengas.`,
		Args: noArgs("uninstall"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			yes, _ := cmd.Flags().GetBool("yes")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			if list, _ := cmd.Flags().GetBool("list"); list {
				dryRun = true
			}
			cfgPath, err := deps.configPath()
			if err != nil {
				return err
			}
			dbPath, err := deps.dataPath()
			if err != nil {
				return err
			}
			cacheDir, err := deps.cacheDir()
			if err != nil {
				return err
			}
			res, err := app.Uninstall(cfgPath, dbPath, cacheDir, yes, dryRun)
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	cmd.Flags().Bool("yes", false, "confirma el borrado: sin ella no se borra nada")
	cmd.Flags().Bool("dry-run", false, "enumera las rutas y su tamaño sin borrar nada")
	cmd.Flags().Bool("list", false, "igual que --dry-run")
	return cmd
}
