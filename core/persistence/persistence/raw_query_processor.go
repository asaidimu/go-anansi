package persistence

import (
	"context"
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v6/core/persistence/base"
	"github.com/asaidimu/go-anansi/v6/core/query"
)

// RegistryEntryProvider defines the interface for retrieving registry entries.
// This is a smaller interface than the full CollectionRegistry, allowing for
// more focused and testable components.
type RegistryEntryProvider interface {
	GetRegistryEntry(ctx context.Context, name string) (*base.RegistryEntry, error)
}

type rawQueryProcessor struct {
	registry RegistryEntryProvider
}

func newRawQueryProcessor(registry RegistryEntryProvider) base.RawQueryProcessor {
	return &rawQueryProcessor{registry: registry}
}

func (p *rawQueryProcessor) ProcessRawQueryTemplate(ctx context.Context, template string, collections map[string]query.RawQueryTarget) (string, error) {
	resolvedTemplate := template
	for placeholderKey, target := range collections {
		// Get the physical collection name from the registry
		entry, err := p.registry.GetRegistryEntry(ctx, target.Collection)
		if err != nil {
			return "", fmt.Errorf("failed to get registry entry for collection '%s': %w", target.Collection, err)
		}

		version := target.Version
		if len(version) == 0 {
			version = entry.ActiveVersion
		}

		physicalCollectionName := entry.Versions[version].Physical
		if len(physicalCollectionName) == 0 {
			return "", fmt.Errorf("failed to get registry entry for collection '%s': %w", target.Collection, err)
		}

		// Replace all occurrences of the placeholder with the physical name
		placeholder := fmt.Sprintf("{{collections.%s}}", placeholderKey)
		resolvedTemplate = strings.ReplaceAll(resolvedTemplate, placeholder, physicalCollectionName)
	}
	return resolvedTemplate, nil
}
