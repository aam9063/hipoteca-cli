package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aam9063/hipoteca-cli/internal/app"
	"github.com/aam9063/hipoteca-cli/internal/bde"
	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/output"
	"github.com/spf13/cobra"
)

// newIndexCmd monta el grupo `index`: list, show y latest. Los rangos de
// --range se traducen aquí al contrato de la API (decisión 4 de CMD-01).
func newIndexCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Índices de referencia del Banco de España: lista, último valor e histórico",
		// Igual que la raíz: sin subcomando no hay resultado, y el único
		// output es el error de uso que main convierte en sobre (P-01).
		RunE: func(_ *cobra.Command, _ []string) error {
			return errs.New(errs.KindUsage,
				"esperando un subcomando de \"hipoteca index\": no hay ningún resultado que mostrar",
				"ejecute \"hipoteca index --help\" para ver los subcomandos disponibles")
		},
	}
	cmd.AddCommand(
		newIndexListCmd(deps),
		newIndexShowCmd(deps),
		newIndexLatestCmd(deps),
	)
	return cmd
}

// newIndexListCmd: todos los índices del catálogo con su último valor.
func newIndexListCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Todos los índices del catálogo con su último valor publicado",
		Args:  noArgs("index list"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := deps.runtime()
			if err != nil {
				return err
			}
			res, err := app.IndexList(cmd.Context(), rt.Source, rt.Catalog)
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
}

// newIndexShowCmd: el histórico mensual de un índice para un rango.
func newIndexShowCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Histórico mensual de un índice",
		Args:  exactArgs("index show", 1, "indica el id del índice; \"hipoteca index list\" muestra los disponibles"),
		RunE: func(cmd *cobra.Command, args []string) error {
			rangoValue, _ := cmd.Flags().GetString("range")
			rango, err := parseRange(rangoValue)
			if err != nil {
				return err
			}
			rt, err := deps.runtime()
			if err != nil {
				return err
			}
			res, err := app.IndexHistory(cmd.Context(), rt.Source, rt.Catalog, args[0], rango)
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
	// S6 (AGENT-02): el texto es estático a propósito — sin el año en curso,
	// que cambiaría cada 1 de enero y desfasaría la ayuda y el documento
	// generado. parseRange sigue aceptando el año real.
	cmd.Flags().String("range", "30M",
		"rango del histórico: 30M, 60M, MAX o un año con datos (entre 1990 y el año en curso)")
	return cmd
}

// newIndexLatestCmd: el último valor de uno o varios índices.
func newIndexLatestCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "latest <id>...",
		Short: "Último valor publicado de uno o varios índices",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errs.New(errs.KindUsage,
					"no se ha indicado ningún índice",
					"pasa uno o más ids del catálogo; \"hipoteca index list\" los muestra")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := deps.runtime()
			if err != nil {
				return err
			}
			res, err := app.LatestIndices(cmd.Context(), rt.Source, rt.Catalog, args)
			if err != nil {
				return err
			}
			return writeResult(cmd, res)
		},
	}
}

// parseRange traduce el --range del usuario al rango del contrato
// (docs/series.md §2.3): 30M, 60M y MAX se pasan tal cual; un año con datos
// (1990..año en curso) viaja como año. Cualquier otra cosa es usage/2 con la
// lista de valores aceptados: nada de recortes silenciosos.
func parseRange(value string) (bde.Rango, error) {
	switch value {
	case "30M":
		return bde.Rango30M, nil
	case "60M":
		return bde.Rango60M, nil
	case "MAX":
		return bde.RangoMAX, nil
	}
	if year, err := strconv.Atoi(value); err == nil {
		if year >= 1990 && year <= time.Now().Year() {
			return bde.RangoAnio(year)
		}
	}
	return bde.Rango{}, errs.New(errs.KindUsage,
		fmt.Sprintf("el rango %q no es correcto", value),
		"usa 30M, 60M, MAX o un año con datos (entre 1990 y "+strconv.Itoa(time.Now().Year())+")")
}

// writeResult escribe el resultado por stdout: una llamada, un valor JSON
// (P-01). output.WriteJSON es el único escritor del CLI.
func writeResult(cmd *cobra.Command, v any) error {
	if err := output.WriteJSON(cmd.OutOrStdout(), v); err != nil {
		return fmt.Errorf("escribiendo el resultado: %w", err)
	}
	return nil
}

// noArgs rechaza cualquier argumento posicional con un error de uso del
// contrato (código 2), no del binario (código 1).
func noArgs(use string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > 0 {
			return errs.New(errs.KindUsage,
				fmt.Sprintf("\"hipoteca %s\" no acepta argumentos: se ha recibido %q", use, args[0]), "")
		}
		return nil
	}
}

// exactArgs exige exactamente n argumentos con un error de uso del contrato.
func exactArgs(use string, n int, hint string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) != n {
			return errs.New(errs.KindUsage,
				fmt.Sprintf("\"hipoteca %s\" espera %d argumento(s) y ha recibido %d", use, n, len(args)), hint)
		}
		return nil
	}
}

// maxArgs acepta como mucho n argumentos con un error de uso del contrato.
func maxArgs(n int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) > n {
			return errs.New(errs.KindUsage,
				fmt.Sprintf("se esperaban como mucho %d argumento(s) y ha recibido %d: %q", n, len(args), strings.Join(args, " ")), "")
		}
		return nil
	}
}
