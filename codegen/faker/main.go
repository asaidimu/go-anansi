package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

func main() {
	seed := flag.Int64("seed", 42, "random seed for reproducibility")
	pretty := flag.Bool("pretty", true, "pretty-print JSON")
	count := flag.Int("count", 1, "number of records to generate")
	dir := flag.String("dir", "", "directory to scan for .schema.json files")
	flag.Parse()

	if *dir == "" && flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: faker [--seed 42] [--count 1] [--pretty] --dir <path>")
		fmt.Fprintln(os.Stderr, "   or: faker [--seed 42] [--count 1] [--pretty] <file1> [file2...]")
		os.Exit(1)
	}

	var files []string
	if *dir != "" {
		filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(info.Name(), ".schema.json") {
				files = append(files, path)
			}
			return nil
		})
	} else {
		files = flag.Args()
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "no schema files found")
		os.Exit(1)
	}

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read %s: %v\n", path, err)
			continue
		}

		schema, err := definition.FromJSON(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", path, err)
			continue
		}

		gen := NewFakeGenerator(schema, *seed)
		results := make([]map[string]any, *count)

		for i := 0; i < *count; i++ {
			results[i] = gen.Generate()
		}

		var out []byte
		if *pretty {
			out, err = json.MarshalIndent(results, "", "  ")
		} else {
			out, err = json.Marshal(results)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "marshal %s: %v\n", path, err)
			continue
		}

		if len(files) == 1 {
			fmt.Println(string(out))
		} else {
			base := strings.TrimSuffix(filepath.Base(path), ".schema.json")
			fmt.Printf("--- %s ---\n%s\n", base, string(out))
		}
	}
}
