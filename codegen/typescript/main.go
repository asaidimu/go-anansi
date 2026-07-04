package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

func main() {
	dir := flag.String("dir", "", "directory to scan for .schema.json files")
	outDir := flag.String("out", "", "output directory (default: same as input)")
	flag.Parse()

	if *dir == "" && flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: typescript --dir <path> [--out <path>]")
		fmt.Fprintln(os.Stderr, "   or: typescript <file1> [file2...] [--out <path>]")
		os.Exit(1)
	}

	if *dir != "" {
		err := processDir(*dir, *outDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	out := *outDir
	if out != "" {
		os.MkdirAll(out, 0755)
	}
	for _, arg := range flag.Args() {
		err := processFile(arg, out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error processing %s: %v\n", arg, err)
			os.Exit(1)
		}
	}
}

func processDir(dir, outDir string) error {
	if outDir != "" {
		os.MkdirAll(outDir, 0755)
	}
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".schema.json") {
			return nil
		}
		out := outDir
		if out == "" {
			out = filepath.Dir(path)
		}
		return processFile(path, out)
	})
}

func processFile(schemaPath, outDir string) error {
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	schema, err := definition.FromJSON(data)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	gen := NewTSGenerator(schema)
	ts := gen.Generate()

	outPath := outPathFor(schemaPath, outDir)
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(outPath, []byte(ts), 0644); err != nil {
		return fmt.Errorf("write: %w", err)
	}

	rel, _ := filepath.Rel(outDir, outPath)
	if outDir == "" {
		rel = filepath.Base(outPath)
	}
	fmt.Printf("generated %s\n", rel)
	return nil
}

func outPathFor(schemaPath, outDir string) string {
	base := filepath.Base(schemaPath)
	outName := strings.TrimSuffix(base, ".schema.json")
	if outName == base {
		outName = strings.TrimSuffix(base, ".json")
	}
	outName += ".ts"
	if outDir != "" {
		return filepath.Join(outDir, outName)
	}
	return filepath.Join(filepath.Dir(schemaPath), outName)
}
