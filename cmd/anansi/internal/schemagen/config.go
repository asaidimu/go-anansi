package schemagen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Schema   SchemaConfig   `json:"schema"`
	TSGen    TSGenConfig    `json:"tsgen"`
}

type SchemaConfig struct {
	Glob         string `json:"glob"`
	Lockfile     string `json:"lockfile"`
	MigrationsDir string `json:"migrations_dir"`
}

type TSGenConfig struct {
	Out string `json:"out"`
}

func DefaultConfig() *Config {
	return &Config{
		Schema: SchemaConfig{
			Glob:         "schemas/**/*.schema.json",
			Lockfile:     "schemas.lock.json",
			MigrationsDir: "migrations/",
		},
		TSGen: TSGenConfig{
			Out: "types.ts",
		},
	}
}

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var fileCfg Config
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	// Override defaults with file values
	if fileCfg.Schema.Glob != "" {
		cfg.Schema.Glob = fileCfg.Schema.Glob
	}
	if fileCfg.Schema.Lockfile != "" {
		cfg.Schema.Lockfile = fileCfg.Schema.Lockfile
	}
	if fileCfg.Schema.MigrationsDir != "" {
		cfg.Schema.MigrationsDir = fileCfg.Schema.MigrationsDir
	}
	if fileCfg.TSGen.Out != "" {
		cfg.TSGen.Out = fileCfg.TSGen.Out
	}

	return cfg, nil
}

func FindConfig() string {
	dir, _ := os.Getwd()
	for {
		candidate := filepath.Join(dir, "anansi.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
