package cli

import (
	"fmt"
	"strings"

	"github.com/aam9063/hipoteca-cli/internal/errs"
	"github.com/aam9063/hipoteca-cli/internal/model"
	"github.com/aam9063/hipoteca-cli/internal/output"
	"github.com/aam9063/hipoteca-cli/internal/skills"
	"github.com/spf13/cobra"
)

// newSkillsCmd monta el grupo `skills` (AGENT-01, S1/S2): la guía embebida
// para agentes. No necesita config, ni red, ni préstamos: un agente sin
// contexto la puede leer antes que nada.
func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Recetas embebidas para agentes: qué comando ejecutar y cómo",
		// Igual que la raíz: sin subcomando no hay resultado, y el único
		// output es el error de uso que main convierte en sobre (P-01).
		RunE: func(_ *cobra.Command, _ []string) error {
			return errs.New(errs.KindUsage,
				"esperando un subcomando de \"hipoteca skills\": no hay ningún resultado que mostrar",
				"ejecute \"hipoteca skills --help\" para ver los subcomandos disponibles")
		},
	}
	cmd.AddCommand(newSkillsListCmd(), newSkillsGetCmd())
	return cmd
}

// skillsListOutput es el contrato de `skills list` (S1): el catálogo con su
// schema_version, id, título y una línea de para qué sirve cada receta.
type skillsListOutput struct {
	SchemaVersion int              `json:"schema_version"`
	Skills        []skills.Summary `json:"skills"`
}

// newSkillsListCmd: las recetas disponibles con su id y su resumen.
func newSkillsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Las recetas disponibles, con su id y una línea de para qué sirven",
		Args:  noArgs("skills list"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := skillsListOutput{
				SchemaVersion: model.SchemaVersion,
				Skills:        skills.List(),
			}
			if err := output.WriteJSON(cmd.OutOrStdout(), out); err != nil {
				return fmt.Errorf("escribiendo el resultado: %w", err)
			}
			return nil
		},
	}
}

// newSkillsGetCmd: la receta completa en texto plano (S2). Es LA excepción
// text/plain del contrato (arranque §6.1, decisión 7): una receta es
// prosa para un agente, no un valor JSON. Un id desconocido es
// not_found/4 con la lista de ids válidos en el hint y stdout vacío.
func newSkillsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "La receta completa, en texto plano listo para ejecutar",
		Args:  exactArgs("skills get", 1, "ejemplo: hipoteca skills get next-revision; \"hipoteca skills list\" muestra los ids"),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, ok := skills.Get(args[0])
			if !ok {
				return errs.New(errs.KindNotFound,
					fmt.Sprintf("no existe ninguna skill llamada %q", args[0]),
					"skills disponibles: "+strings.Join(skills.IDs(), ", "))
			}
			// Text/plain con un salto final: la única salida del CLI que no
			// es JSON, junto a la ayuda explícita (decisión 7).
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), s.Body); err != nil {
				return fmt.Errorf("escribiendo la receta: %w", err)
			}
			return nil
		},
	}
}
