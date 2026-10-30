package cli

import (
	"strings"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/spf13/cobra"
)

// newConfigCmd monta el grupo `config` (AGENT-01, S6/S7): show, get y set.
// Cierra el hueco de WATCH-01: declarar un sink ya no obliga a editar el
// TOML a mano. `set` valida antes de escribir y guarda con la disciplina
// atómica y 0600 de internal/config. Ninguna salida incluye un secreto: la
// URL de un sink con credenciales se muestra redactada (S7).
func newConfigCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "La configuración local: verla y cambiar overrides y sinks",
		// Igual que la raíz: sin subcomando no hay resultado (P-01).
		RunE: func(_ *cobra.Command, _ []string) error {
			return errs.New(errs.KindUsage,
				"esperando un subcomando de \"hipoteca config\": no hay ningún resultado que mostrar",
				"ejecute \"hipoteca config --help\" para ver los subcomandos disponibles")
		},
	}
	cmd.AddCommand(
		newConfigShowCmd(deps),
		newConfigGetCmd(deps),
		newConfigSetCmd(deps),
	)
	return cmd
}

// newConfigShowCmd: la config completa, con las URLs de los sinks
// redactadas.
func newConfigShowCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "La configuración completa (las URLs de los sinks salen redactadas)",
		Args:  noArgs("config show"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			res, err := app.ConfigShow(path)
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
}

// newConfigGetCmd: el valor de una clave puntual (indices.<id>,
// sinks.<nombre>.type|url), redactada si es una URL de sink.
func newConfigGetCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "get <clave>",
		Short: "El valor de una clave: indices.<id>, sinks.<nombre>.type o sinks.<nombre>.url",
		Args:  exactArgs("config get", 1, "ejemplo: hipoteca config get sinks.ntfy.url; \"hipoteca config show\" muestra todo"),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			res, err := app.ConfigGet(path, args[0])
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
}

// newConfigSetCmd: declara o cambia una clave. Para las claves de índices el
// id se valida contra el catálogo (el runtime lo trae); los sinks se
// validan por forma. Un préstamo nunca se edita desde aquí: el hint manda a
// `loan edit`.
func newConfigSetCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "set <clave> <valor>",
		Short: "Declara o cambia una clave: un sink nuevo o un código de serie",
		Long: `Declara o cambia una clave de la configuración. Las claves admitidas:

  sinks.<nombre>.url     la URL del webhook de avisos (declara el sink si no existe)
  sinks.<nombre>.type    el tipo del sink; en v1, "webhook"
  indices.<id>           el código de serie del Banco para un índice del catálogo

El valor se valida ANTES de escribir: una URL que no sea http(s) con host, un
tipo que no sea webhook o un índice fuera del catálogo son errores de uso y la
config queda intacta. Los préstamos se editan con "hipoteca loan add/edit",
no desde aquí.`,
		Args: exactArgs("config set", 2, "ejemplo: hipoteca config set sinks.ntfy.url https://ntfy.sh/mi-topic"),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			// El catálogo sólo hace falta para los overrides de índices:
			// `sinks.*` no toca la red ni necesita runtime. Un fallo del
			// runtime se propaga tal cual: es del contrato (config ilegible,
			// override inválido).
			var catalog *bde.Catalog
			if strings.HasPrefix(key, "indices.") {
				rt, err := deps.runtime()
				if err != nil {
					return err
				}
				catalog = rt.Catalog
			}
			path, err := deps.configPath()
			if err != nil {
				return err
			}
			res, err := app.ConfigSet(path, catalog, key, value)
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
}
