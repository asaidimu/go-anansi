package schemagen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/asaidimu/go-anansi/v8/codegen/golang"
)

type Config struct {
	Schema SchemaConfig `json:"schema"`
	TSGen  TSGenConfig  `json:"tsgen"`
	GoGen  GoGenConfig  `json:"gogen"`
}

type SchemaConfig struct {
	Glob          string `json:"glob"`
	Lockfile      string `json:"lockfile"`
	MigrationsDir string `json:"migrations_dir"`
}

type TSGenConfig struct {
	Out string `json:"out"`
}

// NameRuleJSON is the JSON-serializable form of a name rule.
type NameRuleJSON struct {
	Pattern string `json:"pattern"`
	Prefix  string `json:"prefix"`
}

// GoGenConfig holds configuration specific to Go code generation.
type GoGenConfig struct {
	Tags               golang.TagConfig
	ScopedPackages     bool
	NameRules          []golang.NameRule
}

// UnmarshalJSON implements json.Unmarshaler for GoGenConfig.
// It deserializes name_rules from string-pattern JSON and compiles them.
func (g *GoGenConfig) UnmarshalJSON(data []byte) error {
	type inner struct {
		Tags               golang.TagConfig `json:"tags"`
		PointerForOptional bool             `json:"pointer_for_optional"`
		ScopedPackages     bool             `json:"scoped"`
		NameRules          []NameRuleJSON   `json:"name_rules"`
	}
	var raw inner
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	g.Tags = raw.Tags
	g.ScopedPackages = raw.ScopedPackages
	g.NameRules = make([]golang.NameRule, 0, len(raw.NameRules))
	for _, nr := range raw.NameRules {
		re, err := regexp.Compile(nr.Pattern)
		if err != nil {
			return fmt.Errorf("invalid name rule pattern %q: %w", nr.Pattern, err)
		}
		g.NameRules = append(g.NameRules, golang.NameRule{Pattern: re, Prefix: nr.Prefix})
	}
	return nil
}

func DefaultConfig() *Config {
	return &Config{
		Schema: SchemaConfig{
			Glob:          "schemas/**/*.schema.json",
			Lockfile:      "schemas.lock.json",
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
	if len(fileCfg.GoGen.NameRules) > 0 {
		cfg.GoGen = fileCfg.GoGen
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
