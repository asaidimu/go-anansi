package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/asaidimu/go-anansi/v7/cmd/anansi/internal/schemagen"
)

// Version and Release are set at build time via ldflags.
// See Makefile: -X main.Version=$(VERSION) -X main.Release=$(RELEASE)
var Version = "dev"
var Release = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: anansi <command> [args]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "commands:")
		fmt.Fprintln(os.Stderr, "  scaffold          Create a new anansi project with default config")
		fmt.Fprintln(os.Stderr, "  schema migrate    Generate migration files from schema changes")
		fmt.Fprintln(os.Stderr, "  schema new        Create a blank schema file")
		fmt.Fprintln(os.Stderr, "  schema squash     Consolidate intermediate migrations into one")
		fmt.Fprintln(os.Stderr, "  schema normalize  Normalize schema IDs to UUID v7")
		fmt.Fprintln(os.Stderr, "  schema typescript Generate TypeScript types for all schemas")
		fmt.Fprintln(os.Stderr, "  schema agents     Write comprehensive AGENTS.md reference doc")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "scaffold":
		scaffoldCmd(os.Args[2:])
	case "schema":
		schemaCmd(os.Args[2:])
	case "version":
		fmt.Println(Version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func scaffoldCmd(args []string) {
	fs := flag.NewFlagSet("scaffold", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "print what would be done without making changes")
	fs.Parse(args)

	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}
	if err := schemagen.RunScaffold(dir, *dryRun, Release); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
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

func schemaCmd(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: anansi schema <subcommand>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "subcommands:")
		fmt.Fprintln(os.Stderr, "  migrate    Generate migration files from schema changes")
		fmt.Fprintln(os.Stderr, "  new        Create a blank schema file")
		fmt.Fprintln(os.Stderr, "  squash     Consolidate intermediate migrations")
		fmt.Fprintln(os.Stderr, "  normalize  Normalize schema file IDs to UUID v7")
		fmt.Fprintln(os.Stderr, "  typescript Generate TypeScript types for all schemas (single file)")
		fmt.Fprintln(os.Stderr, "  agents     Write comprehensive AGENTS.md reference doc")
		os.Exit(1)
	}

	switch args[0] {
	case "migrate":
		migrateCmd(args[1:])
	case "new":
		newSchemaCmd(args[1:])
	case "squash":
		squashCmd(args[1:])
	case "normalize":
		normalizeCmd(args[1:])
	case "typescript":
		typescriptCmd(args[1:])
	case "agents":
		agentsCmd(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown schema subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func migrateCmd(args []string) {
	cfg := loadCfg()
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	var (
		glob     string
		lockfile string
		out      string
		check    bool
		dryRun   bool
	)
	fs.StringVar(&glob, "glob", "", "glob pattern for schema files (overrides config)")
	fs.StringVar(&lockfile, "lockfile", "", "lockfile path (overrides config)")
	fs.StringVar(&out, "out", "", "output directory for generated migration files (overrides config)")
	fs.BoolVar(&check, "check", false, "exit with non-zero if migrations need regeneration")
	fs.BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	fs.Parse(args)

	if glob != "" {
		cfg.Schema.Glob = glob
	}
	if lockfile != "" {
		cfg.Schema.Lockfile = lockfile
	}
	if out != "" {
		cfg.Schema.MigrationsDir = out
	}

	if err := schemagen.RunGen(cfg, check, dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newSchemaCmd(args []string) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	dir := fs.String("dir", ".", "output directory for the new schema file")
	dryRun := fs.Bool("dry-run", false, "print what would be done without making changes")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: anansi schema new <name> [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "flags:")
		fmt.Fprintln(os.Stderr, "  --dir     output directory (default .)")
		fmt.Fprintln(os.Stderr, "  --dry-run print what would be done without making changes")
		os.Exit(1)
	}

	name := fs.Arg(0)
	if err := schemagen.RunNewSchema(name, *dir, *dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func squashCmd(args []string) {
	fs := flag.NewFlagSet("squash", flag.ExitOnError)
	var (
		lockfile string
		out      string
		dryRun   bool
	)
	fs.StringVar(&lockfile, "lockfile", "", "lockfile path (overrides config)")
	fs.StringVar(&out, "out", "", "migration output directory (overrides config)")
	fs.BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: anansi schema squash <collection> [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "flags:")
		fmt.Fprintln(os.Stderr, "  --lockfile  lockfile path (overrides config)")
		fmt.Fprintln(os.Stderr, "  --out       migration output directory (overrides config)")
		fmt.Fprintln(os.Stderr, "  --dry-run   print what would be done without making changes")
		os.Exit(1)
	}

	cfg := loadCfg()
	collection := fs.Arg(0)

	if lockfile != "" {
		cfg.Schema.Lockfile = lockfile
	}
	if out != "" {
		cfg.Schema.MigrationsDir = out
	}

	if err := schemagen.RunSquash(cfg, collection, dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func normalizeCmd(args []string) {
	fs := flag.NewFlagSet("normalize", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "print what would be done without making changes")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: anansi schema normalize <path>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "normalize schema file IDs to UUID v7 in-place")
		os.Exit(1)
	}

	if err := schemagen.RunNormalize(fs.Arg(0), *dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func typescriptCmd(args []string) {
	cfg := loadCfg()
	fs := flag.NewFlagSet("typescript", flag.ExitOnError)
	var (
		glob   string
		out    string
		dryRun bool
	)
	fs.StringVar(&glob, "glob", "", "glob pattern for schema files (overrides config)")
	fs.StringVar(&out, "out", "", "output TypeScript file (overrides config)")
	fs.BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	fs.Parse(args)

	if glob != "" {
		cfg.Schema.Glob = glob
	}
	if out != "" {
		cfg.TSGen.Out = out
	}

	if err := schemagen.RunTSGen(cfg, dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func agentsCmd(args []string) {
	fs := flag.NewFlagSet("agents", flag.ExitOnError)
	var (
		out    string
		dryRun bool
	)
	fs.StringVar(&out, "out", ".", "output directory for AGENTS.md")
	fs.BoolVar(&dryRun, "dry-run", false, "print what would be done without making changes")
	fs.Parse(args)

	if err := schemagen.RunAgents(out, dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
