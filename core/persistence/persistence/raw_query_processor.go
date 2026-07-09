package persistence

import (
	"context"
	"fmt"
	"strings"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/query"
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
			return "", common.SystemErrorFrom(err, base.ErrRawQueryProcessorRegistryLookupFailed.Code).
				WithMessage(fmt.Sprintf("failed to get registry entry for collection '%s'", target.Collection)).
				WithPath(target.Collection)
		}

		version := target.Version
		if len(version) == 0 {
			version = entry.ActiveVersion.String()
		}

		physicalCollectionName := entry.Versions[version].Physical
		if len(physicalCollectionName) == 0 {
			return "", common.SystemErrorFrom(base.ErrFailedToResolvePhysicalName, base.ErrRawQueryProcessorPhysicalNameResolutionFailed.Code).
				WithMessage(fmt.Sprintf("failed to resolve physical name for collection '%s' version '%s'", target.Collection, version)).
				WithPath(fmt.Sprintf("%s:%s", target.Collection, version))
		}
		// Replace all occurrences of the placeholder with the physical name
		placeholder := fmt.Sprintf("{{collections.%s}}", placeholderKey)
		resolvedTemplate = strings.ReplaceAll(resolvedTemplate, placeholder, physicalCollectionName)
	}
	return resolvedTemplate, nil
}
