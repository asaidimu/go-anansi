package base

import (
	"context"

	"github.com/asaidimu/go-anansi/v7/core/common"
	"github.com/asaidimu/go-anansi/v7/core/data"
	"github.com/asaidimu/go-anansi/v7/core/schema/definition"
)

type MigrationPhase string

const (
	// PhaseSchemaOnly applies only DDL that the backend supports in-place
	// (e.g., add/drop column, create/drop index). No data copy is performed.
	// If the backend cannot perform some DDL in-place, the migration fails
	// and the caller should retry with PhaseDDLOrFull or PhaseFull.
	PhaseSchemaOnly MigrationPhase = "schema_only"

	// PhaseDDL attempts in-place DDL for every change in the diff. For any
	// change the backend cannot do in-place (e.g., rename column on SQLite),
	// it falls back to a full table copy + transform for that collection.
	PhaseDDL MigrationPhase = "ddl"

	// PhaseFull always creates a new physical collection and copies/transforms
	// all data. This is the safest fallback and works on every backend.
	PhaseFull MigrationPhase = "full"
)

type MigrationPlan struct {
	Description string                `json:"description"`
	Target      *definition.Schema    `json:"target"`
	Diff        *definition.SchemaDiff `json:"diff,omitempty"`
	VersionBump definition.VersionBump `json:"versionBump,omitempty"`
	Phase       MigrationPhase         `json:"phase"`
	Transformer TransformerFunc        `json:"-"`
}

type TransformerFunc func(ctx context.Context, sourceDoc data.Document) (data.Document, error)

func NewSchemaOnlyMigration(target *definition.Schema, description string) *MigrationPlan {
	return &MigrationPlan{
		Description: description,
		Target:      target,
		Phase:       PhaseSchemaOnly,
	}
}

func NewFullMigration(target *definition.Schema, description string, transformer TransformerFunc) *MigrationPlan {
	return &MigrationPlan{
		Description: description,
		Target:      target,
		Phase:       PhaseFull,
		Transformer: transformer,
	}
}

func (p *MigrationPlan) ComputeDiff(currentSchema *definition.Schema) error {
	if p.Target == nil {
		return nil
	}
	diff, err := definition.Diff(currentSchema, p.Target)
	if err != nil {
		return err
	}
	p.Diff = diff
	if p.VersionBump == definition.BumpNone {
		p.VersionBump = definition.VersionImpact(diff)
	}
	return nil
}

func (p *MigrationPlan) TargetVersion(current *common.Version) *common.Version {
	bumped := p.VersionBump.Apply(*current)
	return &bumped
}

func (p *MigrationPlan) NeedsDataMigration() bool {
	return p.Phase == PhaseFull && p.Transformer != nil
}

// ResolvePhase determines the migration phase at runtime based on the diff
// and the backend's DDL capabilities. If Phase is already set (by the user or
// generated code), it is honored. Otherwise it chooses:
//   - PhaseSchemaOnly when every change can be done in-place via DDL
//   - PhaseDDL when some changes require capabilities the backend lacks
//   - PhaseFull when a Transformer func is provided
//
// canApplyInPlace is a caller-provided function that checks whether the given
// diff can be applied using only in-place DDL on the target backend.
func (p *MigrationPlan) ResolvePhase(canApplyInPlace func(*definition.SchemaDiff) bool) {
	if p.Phase != "" {
		return
	}
	if p.Diff == nil {
		return
	}
	if p.Transformer != nil {
		p.Phase = PhaseFull
		return
	}
	if canApplyInPlace(p.Diff) {
		p.Phase = PhaseSchemaOnly
	} else {
		p.Phase = PhaseDDL
	}
}
