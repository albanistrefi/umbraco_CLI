package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"umbraco-cli/internal/skills"
	"umbraco-cli/internal/validate"
)

// CLIVersion identifies the published umbraco-cli release. It is embedded into generated SKILL.md
// frontmatter and surfaced via `umbraco --version`.
const CLIVersion = "0.2.7"

func RegisterGenerateSkills(root *cobra.Command, deps Dependencies) {
	var outputDir string
	var filter string

	cmd := &cobra.Command{
		Use:   "generate-skills",
		Short: "Generate SKILL.md files from CLI command metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validate.String(outputDir); err != nil {
				return fmt.Errorf("invalid output directory: %w", err)
			}

			if err := skills.Generate(root.Root(), outputDir, filter, CLIVersion); err != nil {
				return err
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Skills written to %s/\n", outputDir)
			return nil
		},
	}

	cmd.Flags().StringVar(&outputDir, "output-dir", "skills/cli", "Directory to write generated skills")
	cmd.Flags().StringVar(&filter, "filter", "", "Only generate skills matching this substring")
	root.AddCommand(cmd)
}
