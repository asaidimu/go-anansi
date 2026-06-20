package schema

import (
	"embed"
	"fmt"
	"io/fs"
	"sync"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/asaidimu/go-anansi/v6/core/schema/definition"
)

//go:embed models/*.json
var schemasFS embed.FS

var (
	schemas     []*definition.Schema
	schemasOnce sync.Once
	schemasErr  error
)

// GetSchemas returns all schema definitions, loading them from the embedded filesystem on the first call.
func GetSchemas() ([]*definition.Schema, error) {
	schemasOnce.Do(func() {
		loadedSchemas := []*definition.Schema{}
		err := walkSchemas(func(def *definition.Schema) error {
			loadedSchemas = append(loadedSchemas, def)
			return nil
		})

		if err != nil {
			schemasErr = err
			return
		}
		schemas = loadedSchemas
	})

	return schemas, schemasErr
}

func walkSchemas(callback func(*definition.Schema) error) error {
	dir, err := schemasFS.ReadDir("models")
	if err != nil {
		return err
	}

	for _, entry := range dir {
		if !entry.Type().IsRegular() {
			continue
		}
		name := fmt.Sprintf("models/%s", entry.Name())
		bytes, err := fs.ReadFile(schemasFS, name)

		if err != nil {
			return common.SystemErrorFrom(err).WithMessagef("Failed to read %s", name).WithPath(name)
		}
		schemaDef, err := definition.FromJSON(bytes)
		if err != nil {
			return common.SystemErrorFrom(err).WithPath(name)
		}

		if err := callback(schemaDef); err != nil {
			return err
		}
	}

	return nil
}
