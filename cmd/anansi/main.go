package main

import (
	"fmt"
	"os"

	"github.com/asaidimu/go-anansi/v8/cmd/anansi/internal/schemagen"
	"github.com/spf13/cobra"
)

var Version = "dev"
var Release = "dev"

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
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "scaffold [dir]",
		Short: "Create a new anansi project with default config",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return schemagen.RunScaffold(dir, dryRun, Release)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
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

// --- codegen ---

func codegenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codegen",
		Short: "Generate code and data from schemas",
	}

	cmd.AddCommand(codegenTypescriptCmd())
	cmd.AddCommand(codegenFakerCmd())
	cmd.AddCommand(codegenAgentsCmd())

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

func codegenAgentsCmd() *cobra.Command {
	var out string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "agents",
		Short: "Write comprehensive AGENTS.md reference doc",
		RunE: func(cmd *cobra.Command, args []string) error {
			return schemagen.RunAgents(out, dryRun)
		},
	}

	cmd.Flags().StringVar(&out, "out", ".", "output directory for AGENTS.md")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
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
			return schemagen.RunNormalize(args[0], dryRun)
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
