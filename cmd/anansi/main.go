package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/asaidimu/go-anansi/v8/cmd/anansi/internal"
	"github.com/asaidimu/go-anansi/v8/codegen/golang"
	"github.com/spf13/cobra"
)

var Version = "dev"
var Release = "dev"

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		mv := info.Main.Version
		if mv != "" && mv != "(devel)" {
			Version = mv
			Release = mv
		}
	}
}

func main() {
	rootCmd := &cobra.Command{
		Use:     "anansi",
		Short:   "Anansi CLI — schema-aware document persistence toolkit",
		Version: Version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	rootCmd.AddCommand(versionCmd())
	rootCmd.AddCommand(scaffoldCmd())
	rootCmd.AddCommand(agentsCmd())
	rootCmd.AddCommand(migrateCmd())
	rootCmd.AddCommand(codegenCmd())
	rootCmd.AddCommand(schemaCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(Version)
			return nil
		},
	}
}

func scaffoldCmd() *cobra.Command {
	var existing, noInteractive, dryRun bool
	var schemasDir, migrationsDir, lockfile string

	cmd := &cobra.Command{
		Use:   "scaffold [dir]",
		Short: "Create a new anansi project or add anansi to an existing module",
		Long: `Scaffold an anansi project.

Runs interactively when stdin is a terminal (suppress with --no-interactive): it
asks where to put the project, whether to create a standalone app or add anansi
to an existing Go module, and how you want the project organised (schemas dir,
migrations dir, lockfile). Every prompt has a default you can accept as-is or
change — the CLI defaults are just starting points. --no-interactive applies the
defaults; use --existing to add anansi to an existing module without creating
go.mod/main.go, and --schemas-dir/--migrations-dir/--lockfile to set the layout
from a script or agent.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}

			if !noInteractive && isInteractive() {
				if len(args) == 0 {
					if err := promptString("Where should anansi live?", ".", &dir); err != nil {
						return err
					}
				}
				if !existing {
					var shape string
					if err := promptSelect("How do you want to add Anansi?", []string{
						"New standalone app (go.mod + main.go + schemas + migrations)",
						"Existing Go module (add schemas + migrations + config only)",
					}, &shape); err != nil {
						return err
					}
					existing = strings.HasPrefix(shape, "Existing")
				}
				if err := promptString("Where do schema files live? (the dir, not a glob)",
					"schemas", &schemasDir); err != nil {
					return err
				}
				if err := promptString("Where should migrations be generated?",
					"migrations", &migrationsDir); err != nil {
					return err
				}
				if err := promptString("Lockfile path (tracks schema versions + IDs)?",
					"schemas.lock.json", &lockfile); err != nil {
					return err
				}
			}

			return schemagen.RunScaffold(schemagen.ScaffoldOptions{
				Dir:           dir,
				Library:       existing,
				DryRun:        dryRun,
				AnansiVersion: Release,
				SchemasDir:    schemasDir,
				MigrationsDir: migrationsDir,
				Lockfile:      lockfile,
			})
		},
	}

	cmd.Flags().BoolVar(&existing, "existing", false, "add anansi to the existing Go module (no go.mod/main.go)")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "never prompt; accept defaults")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	cmd.Flags().StringVar(&schemasDir, "schemas-dir", "", "schemas directory (default \"schemas\")")
	cmd.Flags().StringVar(&migrationsDir, "migrations-dir", "", "migrations directory (default \"migrations\")")
	cmd.Flags().StringVar(&lockfile, "lockfile", "", "lockfile path (default \"schemas.lock.json\")")

	// Back-compat alias for the pre-rename --yes. Hidden: prefer
	// --no-interactive, which describes the behaviour semantically.
	cmd.Flags().BoolVar(&noInteractive, "yes", false, "deprecated: use --no-interactive")
	cmd.Flags().MarkHidden("yes")

	return cmd
}

// --- migrate ---

func migrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Manage database schema migrations",
	}

	cmd.AddCommand(migrateGenerateCmd())
	cmd.AddCommand(migrateSquashCmd())

	return cmd
}

func migrateGenerateCmd() *cobra.Command {
	var glob, lockfile, out string
	var check, dryRun bool

	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate migration files from schema changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadCfg()
			if glob != "" {
				cfg.Schema.Glob = glob
			}
			if lockfile != "" {
				cfg.Schema.Lockfile = lockfile
			}
			if out != "" {
				cfg.Schema.MigrationsDir = out
			}
			return schemagen.RunGen(cfg, check, dryRun)
		},
	}

	cmd.Flags().StringVar(&glob, "glob", "", "glob pattern for schema files (overrides config)")
	cmd.Flags().StringVar(&lockfile, "lockfile", "", "lockfile path (overrides config)")
	cmd.Flags().StringVar(&out, "out", "", "output directory for generated migration files (overrides config)")
	cmd.Flags().BoolVar(&check, "check", false, "exit with non-zero if migrations need regeneration")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	return cmd
}

func migrateSquashCmd() *cobra.Command {
	var lockfile, out string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "squash <collection>",
		Short: "Consolidate intermediate migrations",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadCfg()
			if lockfile != "" {
				cfg.Schema.Lockfile = lockfile
			}
			if out != "" {
				cfg.Schema.MigrationsDir = out
			}
			return schemagen.RunSquash(cfg, args[0], dryRun)
		},
	}

	cmd.Flags().StringVar(&lockfile, "lockfile", "", "lockfile path (overrides config)")
	cmd.Flags().StringVar(&out, "out", "", "migration output directory (overrides config)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	return cmd
}

// --- agents ---

func agentsCmd() *cobra.Command {
	var local, global, dryRun bool

	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Install the bundled Anansi agent skill",
		Long: `Install the Anansi agent skill (SKILL.md, references/, evals/) that ships
with this binary.

Local (default) installs into the current project's git root under
.agents/skills/anansi so project-scoped agent tools load it automatically.
--global installs into the user's agent skills directory instead.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("local") && cmd.Flags().Changed("global") {
				return fmt.Errorf("--local and --global are mutually exclusive")
			}
			target := schemagen.DefaultSkillDir()
			if global {
				var err error
				target, err = schemagen.GlobalSkillDir()
				if err != nil {
					return err
				}
			}
			return schemagen.InstallSkill(target, dryRun)
		},
	}

	cmd.Flags().BoolVar(&local, "local", true, "install into the current project (git root) — default")
	cmd.Flags().BoolVar(&global, "global", false, "install into the user's global agent skills directory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	return cmd
}

// --- codegen ---

func codegenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codegen",
		Short: "Generate code and data from schemas",
	}

	cmd.AddCommand(codegenGolangCmd())
	cmd.AddCommand(codegenTypescriptCmd())
	cmd.AddCommand(codegenFakerCmd())

	return cmd
}

func codegenTypescriptCmd() *cobra.Command {
	var glob, out string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "typescript",
		Short: "Generate TypeScript types for all schemas",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadCfg()
			if glob != "" {
				cfg.Schema.Glob = glob
			}
			if out != "" {
				cfg.TSGen.Out = out
			}
			return schemagen.RunTSGen(cfg, dryRun)
		},
	}

	cmd.Flags().StringVar(&glob, "glob", "", "glob pattern for schema files (overrides config)")
	cmd.Flags().StringVar(&out, "out", "", "output TypeScript file (overrides config)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	return cmd
}

func codegenGolangCmd() *cobra.Command {
	var glob, mode string
	var scoped, noTags, dryRun bool

	cmd := &cobra.Command{
		Use:   "golang [glob...]",
		Short: "Generate Go structs for all schemas",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := loadCfg()
			if glob != "" {
				cfg.Schema.Glob = glob
			}
			if mode != "" {
				m, err := golang.ParseGenerationMode(mode)
				if err != nil {
					return err
				}
				cfg.GoGen.Mode = m
			}
			if cmd.Flags().Changed("scoped") {
				cfg.GoGen.ScopedPackages = scoped
			}
			if noTags {
				cfg.GoGen.Tags = nil
				cfg.GoGen.TagsSet = true
			}
			return schemagen.RunGoGen(cfg, dryRun, args...)
		},
	}

	cmd.Flags().StringVar(&glob, "glob", "", "glob pattern for schema files (overrides config)")
	cmd.Flags().StringVar(&mode, "mode", "", "generation mode: structs, model, or full (overrides config)")
	cmd.Flags().BoolVar(&scoped, "scoped", false, "emit scoped (unexported) model accessors (overrides config)")
	cmd.Flags().BoolVar(&noTags, "no-tags", false, "omit all struct tags (overrides config)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	return cmd
}

func codegenFakerCmd() *cobra.Command {
	var seed int64
	var pretty bool
	var count int
	var dir string

	cmd := &cobra.Command{
		Use:   "faker [schema-files...]",
		Short: "Generate fake data from schema files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return schemagen.RunFaker(seed, count, pretty, dir, args)
		},
	}

	cmd.Flags().Int64Var(&seed, "seed", 42, "random seed for reproducibility")
	cmd.Flags().BoolVar(&pretty, "pretty", true, "pretty-print JSON")
	cmd.Flags().IntVar(&count, "count", 1, "number of records to generate")
	cmd.Flags().StringVar(&dir, "dir", "", "directory to scan for .schema.json files")
	return cmd
}

// --- schema ---

func schemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Create and maintain schema files",
	}

	cmd.AddCommand(schemaNewCmd())
	cmd.AddCommand(schemaNormalizeCmd())

	return cmd
}

func schemaNewCmd() *cobra.Command {
	var dir string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Create a blank schema file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return schemagen.RunNewSchema(args[0], dir, dryRun)
		},
	}

	cmd.Flags().StringVar(&dir, "dir", ".", "output directory for the new schema file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	return cmd
}

func schemaNormalizeCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "normalize <path>",
		Short: "Normalize schema file IDs to UUID v7",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return schemagen.RunNormalize(loadCfg(), args[0], dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	return cmd
}

func loadCfg() *schemagen.Config {
	path := schemagen.FindConfig()
	if path == "" {
		fmt.Fprintln(os.Stderr, "error: no anansi.json found in project tree")
		os.Exit(1)
	}
	cfg, err := schemagen.LoadConfig(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: load config: %v\n", err)
		os.Exit(1)
	}
	return cfg
}
