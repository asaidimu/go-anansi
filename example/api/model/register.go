package model

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/schema"
)

//go:embed */schema.json
var embeddedSchemas embed.FS

var (
	schemaCache = make(map[string]*schema.SchemaDefinition)
	cacheMutex  sync.RWMutex
)

// getFromCache safely retrieves a schema from cache
func getFromCache(modelName string) (*schema.SchemaDefinition, bool) {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()
	cached, exists := schemaCache[modelName]
	return cached, exists
}

// loadSchema loads a schema from filesystem (no caching)
func loadSchema(modelName string) (*schema.SchemaDefinition, error) {
	filePath := fmt.Sprintf("%s/schema.json", modelName)
	data, err := fs.ReadFile(embeddedSchemas, filePath)
	if err != nil {
		return nil, fmt.Errorf("could not read schema for model '%s': %w", modelName, err)
	}

	var schemaObj schema.SchemaDefinition
	err = schemaObj.From(data)
	if err != nil {
		return nil, fmt.Errorf("could not parse schema for model '%s': %w", modelName, err)
	}

	return &schemaObj, nil
}

// loadWithCache safely loads and caches a schema
func loadWithCache(modelName string) (*schema.SchemaDefinition, error) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// Double-check after acquiring write lock
	if cached, exists := schemaCache[modelName]; exists {
		return cached, nil
	}

	// Load from filesystem
	schemaObj, err := loadSchema(modelName)
	if err != nil {
		return nil, err
	}

	// Cache the result
	schemaCache[modelName] = schemaObj
	return schemaObj, nil
}

// GetSchema retrieves a single schema by model name with caching
func GetSchema(modelName string) (*schema.SchemaDefinition, error) {
	// Check cache first
	if cached, exists := getFromCache(modelName); exists {
		return cached, nil
	}

	// Load and cache
	return loadWithCache(modelName)
}

// GetAllSchemas retrieves all schemas, using cache when possible
func GetAllSchemas() ([]schema.SchemaDefinition, error) {
	result := make([]schema.SchemaDefinition, 0)

	err := fs.WalkDir(embeddedSchemas, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && strings.HasSuffix(path, "schema.json") {
			modelName := strings.Split(path, "/")[0]
			schemaObj, err := GetSchema(modelName)
			if err != nil {
				return err
			}
			result = append(result, *schemaObj)
		}
		return nil
	})

	return result, err
}
