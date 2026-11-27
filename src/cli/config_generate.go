package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/kestra-io/kestractl/src/configgen"
	"github.com/spf13/cobra"
)

func newConfigGenerateCommand() *cobra.Command {
	var (
		fromFile      string
		outputDir     string
		printDefaults bool
		dryRun        bool
		force         bool
	)

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a Kestra stack (application.yml, docker-compose.yml, .env) from a JSON file.",
		Long: `Generate a ready-to-run Kestra stack from a JSON description.

Use --print-defaults to get an annotated template, edit it, then run with --from-file.`,
		Example: `  kestractl config generate --print-defaults > stack.json
  kestractl config generate --from-file stack.json --dry-run
  kestractl config generate --from-file stack.json --output-dir ./my-stack`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if printDefaults {
				return configgen.PrintDefaults(cmd.OutOrStdout())
			}
			if fromFile == "" {
				return fmt.Errorf("--from-file is required (use --print-defaults for a template)")
			}
			renderer, err := NewRendererFromFlags(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			return runConfigGenerate(fromFile, outputDir, dryRun, force, renderer)
		},
	}

	cmd.Flags().StringVarP(&fromFile, "from-file", "f", "", "Path to the JSON stack description")
	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Directory to write files into (default: project_name from the file)")
	cmd.Flags().BoolVar(&printDefaults, "print-defaults", false, "Print an annotated JSON template and exit")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing files (rotates the encryption key and JWT secret)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate the file without writing anything")

	return cmd
}

type configGenerateResult struct {
	Status    string   `json:"status"`
	OutputDir string   `json:"output_dir,omitempty"`
	Files     []string `json:"files,omitempty"`
}

func runConfigGenerate(path, outputDir string, dryRun, force bool, renderer *Renderer) error {
	if err := validateOutputFormat(); err != nil {
		return err
	}
	choices, err := configgen.LoadFromFile(path)
	if err != nil {
		return err
	}
	if dryRun {
		return renderGenerateResult(renderer, configGenerateResult{Status: "valid"})
	}
	if outputDir == "" {
		outputDir = choices.ProjectName
	}
	files, err := configgen.Generate(choices, outputDir, force)
	if err != nil {
		return fmt.Errorf("generating files: %w", err)
	}
	return renderGenerateResult(renderer, configGenerateResult{Status: "generated", OutputDir: outputDir, Files: files})
}

func renderGenerateResult(renderer *Renderer, res configGenerateResult) error {
	return renderer.Render(res, func(w *tabwriter.Writer) error {
		if res.Status == "valid" {
			_, err := io.WriteString(w, "Config is valid.\n")
			return err
		}
		fmt.Fprintln(w, "FILE")
		for _, f := range res.Files {
			fmt.Fprintln(w, f)
		}
		return nil
	})
}
