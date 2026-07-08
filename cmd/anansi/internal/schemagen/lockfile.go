package schemagen

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

type Lockfile struct {
	Version string                 `json:"version"`
	Schemas map[string]*SchemaRef `json:"schemas"`
}

type SchemaRef struct {
	Path          string              `json:"path"`
	Hash          string              `json:"hash"`
	Version       string              `json:"version"`
	Schema        *definition.Schema  `json:"schema"`
	MigrationFile string              `json:"migration_file,omitempty"`
	History       []*HistoryEntry     `json:"history,omitempty"`
	SubMigrations []string            `json:"sub_migrations,omitempty"`
}

type HistoryEntry struct {
	Version       string             `json:"version"`
	Schema        *definition.Schema `json:"schema"`
	MigrationFile string             `json:"migration_file"`
}

func ReadLockfile(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Lockfile{
				Version: "1",
				Schemas: make(map[string]*SchemaRef),
			}, nil
		}
		return nil, fmt.Errorf("read lockfile: %w", err)
	}

	var lf Lockfile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parse lockfile: %w", err)
	}
	if lf.Schemas == nil {
		lf.Schemas = make(map[string]*SchemaRef)
	}
	return &lf, nil
}

func WriteLockfile(path string, lf *Lockfile) error {
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lockfile: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write lockfile: %w", err)
	}
	return nil
}

func ContentHash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", h[:])
}

// MigrationUUID extracts the UUID from a migration filename like "uuid_Name_type.go".
func MigrationUUID(filename string) string {
	if idx := strings.IndexByte(filename, '_'); idx > 0 {
		return filename[:idx]
	}
	return filename
}

func backupFile(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(path+".bak", data, 0644)
}
