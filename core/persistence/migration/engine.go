package migration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/query"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

var (
	ErrMigrationInProgress = errors.New("migration already in progress")
	ErrNoChanges           = errors.New("no schema changes detected")
	ErrInvalidVersion      = errors.New("invalid version")
	ErrMigrationNotFound   = errors.New("migration not found")
	ErrAlreadyApplied      = errors.New("migration already applied")
	ErrNotApplied          = errors.New("migration not applied")
	ErrDryRunOnly          = errors.New("operation only valid in dry-run mode")
)

// ChangeImpact classifies a schema change's impact on data.
type ChangeImpact int

const (
	ImpactNone     ChangeImpact = 0
	ImpactSafe     ChangeImpact = 1 // additive: new optional fields, indexes
	ImpactModerate ChangeImpact = 2 // data movement needed: type changes, renames
	ImpactBreaking ChangeImpact = 3 // destructive: field removal, constraint tightening
)

func (c ChangeImpact) String() string {
	switch c {
	case ImpactSafe:
		return "safe"
	case ImpactModerate:
		return "moderate"
	case ImpactBreaking:
		return "breaking"
	default:
		return "none"
	}
}

// ClassifyImpact maps a SchemaDiff's changes into an overall impact level.
func ClassifyImpact(diff *definition.SchemaDiff) ChangeImpact {
	impact := ImpactSafe

	for _, c := range diff.Changes {
		switch c.Kind {
		case definition.FieldRemoved:
			return ImpactBreaking
		case definition.FieldAdded:
			for _, op := range c.Forward {
				if op.Type == definition.OpAdd {
					if f, ok := op.Value.(definition.Field); ok && f.Required {
						return ImpactBreaking
					}
				}
			}
			if impact < ImpactModerate {
				impact = ImpactModerate
			}
		case definition.FieldModified:
			for _, op := range c.Forward {
				if op.Type == definition.OpSet {
					lastSeg := op.Path.Segments[len(op.Path.Segments)-1].Type
					switch lastSeg {
					case definition.PathType, definition.PathRequired, definition.PathUnique,
						definition.PathName, definition.PathDefault, definition.PathFieldSchema:
						return ImpactBreaking
					}
				}
			}
			if impact < ImpactModerate {
				impact = ImpactModerate
			}
		case definition.ConstraintAdded, definition.ConstraintRemoved, definition.SchemaAdded, definition.SchemaRemoved:
			return ImpactBreaking
		case definition.IndexAdded, definition.IndexRemoved:
			if impact < ImpactModerate {
				impact = ImpactModerate
			}
		}
	}

	return impact
}

// MigrationRecord tracks a single migration's full lifecycle.
type MigrationRecord struct {
	ID             string              `json:"id"`
	FromVersion    string              `json:"fromVersion"`
	ToVersion      string              `json:"toVersion"`
	FromSchema     *definition.Schema  `json:"fromSchema"`
	ToSchema       *definition.Schema  `json:"toSchema"`
	Diff           *definition.SchemaDiff `json:"diff"`
	Impact         ChangeImpact        `json:"impact"`
	Checksum       string              `json:"checksum"`
	Status         MigrationStatus     `json:"status"`
	Phases         []base.MigrationPhase `json:"phases"`
	CreatedAt      time.Time             `json:"createdAt"`
	StartedAt      *time.Time            `json:"startedAt,omitempty"`
	CompletedAt    *time.Time            `json:"completedAt,omitempty"`
	Error          *string             `json:"error,omitempty"`
	RollbackOf     *string             `json:"rollbackOf,omitempty"`
}

// MigrationStatus tracks migration lifecycle state.
type MigrationStatus string

const (
	StatusPending    MigrationStatus = "pending"
	StatusRunning    MigrationStatus = "running"
	StatusApplied    MigrationStatus = "applied"
	StatusFailed     MigrationStatus = "failed"
	StatusRolledBack MigrationStatus = "rolled_back"
)

// DryRunResult is a preview of what a migration would do, without executing it.
type DryRunResult struct {
	FromVersion string              `json:"fromVersion"`
	ToVersion   string              `json:"toVersion"`
	Impact      ChangeImpact        `json:"impact"`
	Changes     []ChangeSummary     `json:"changes"`
	Checksum    string              `json:"checksum"`
	Phases      []base.MigrationPhase `json:"phases"`
	Diff        *definition.SchemaDiff `json:"diff"`
}

// ChangeSummary is a human-readable summary of a single change.
type ChangeSummary struct {
	Kind       string `json:"kind"`
	Entity     string `json:"entity"`
	Detail     string `json:"detail"`
	RequiresData bool `json:"requiresData"`
}

// BuildChangeSummaries converts a SchemaDiff into human-readable summaries.
func BuildChangeSummaries(diff *definition.SchemaDiff) []ChangeSummary {
	summaries := make([]ChangeSummary, 0, len(diff.Changes))
	for _, c := range diff.Changes {
		cs := ChangeSummary{
			Kind:   c.Kind.String(),
			Entity: c.EntityId,
		}
		// Determine if data migration is needed
		for _, op := range c.Forward {
			switch op.Type {
			case definition.OpSet:
				lastSeg := op.Path.Segments[len(op.Path.Segments)-1].Type
				switch lastSeg {
				case definition.PathType, definition.PathDefault, definition.PathRequired:
					cs.RequiresData = true
				}
			case definition.OpAdd, definition.OpRemove:
				cs.RequiresData = true
			}
			if cs.RequiresData {
				break
			}
		}
		cs.Detail = summarizeChange(c)
		summaries = append(summaries, cs)
	}
	return summaries
}

func summarizeChange(c definition.SemanticChange) string {
	switch c.Kind {
	case definition.FieldAdded:
		return fmt.Sprintf("add field %q", c.EntityId)
	case definition.FieldRemoved:
		return fmt.Sprintf("remove field %q", c.EntityId)
	case definition.FieldModified:
		return fmt.Sprintf("modify field %q", c.EntityId)
	case definition.IndexAdded:
		return fmt.Sprintf("add index %q", c.EntityId)
	case definition.IndexRemoved:
		return fmt.Sprintf("remove index %q", c.EntityId)
	case definition.IndexModified:
		return fmt.Sprintf("modify index %q", c.EntityId)
	case definition.ConstraintAdded:
		return fmt.Sprintf("add constraint %q", c.EntityId)
	case definition.ConstraintRemoved:
		return fmt.Sprintf("remove constraint %q", c.EntityId)
	case definition.ConstraintModified:
		return fmt.Sprintf("modify constraint %q", c.EntityId)
	case definition.SchemaAdded:
		return fmt.Sprintf("add nested schema %q", c.EntityId)
	case definition.SchemaRemoved:
		return fmt.Sprintf("remove nested schema %q", c.EntityId)
	case definition.SchemaModified:
		return fmt.Sprintf("modify nested schema %q", c.EntityId)
	default:
		return fmt.Sprintf("modify %s %q", c.Kind, c.EntityId)
	}
}

// MigrationEngine coordinates schema diffing, version management, phased
// execution, and migration history. It combines the structural diffing of
// the Go schema definition layer with data migration via DatabaseInteractor.
type MigrationEngine struct {
	registry   base.CollectionRegistry
	migrator   *DefaultDataMigrator
	transforms map[string]Transformer // keyed by "collection:version"

	mu      sync.Mutex
	running bool
	history []*MigrationRecord
}

// NewMigrationEngine creates a new engine with the given registry and database
// interactor. The registry provides schema version management; the interactor
// provides raw document read/write for data migration.
func NewMigrationEngine(
	registry base.CollectionRegistry,
	interactor query.DatabaseInteractor,
) *MigrationEngine {
	return &MigrationEngine{
		registry:   registry,
		migrator:   NewDefaultDataMigrator(interactor, registry),
		transforms: make(map[string]Transformer),
		history:    make([]*MigrationRecord, 0),
	}
}

// RegisterTransform registers a data transformation function for a specific
// collection and destination version. The transformer is called for each
// document when migrating data from the previous version to the new version.
func (e *MigrationEngine) RegisterTransform(collection, version string, t Transformer) {
	key := collection + ":" + version
	e.transforms[key] = t
}

// computeChecksum produces a SHA-256 checksum of the schema diff for integrity
// verification.
func computeChecksum(from, to *definition.Schema, diff *definition.SchemaDiff) string {
	h := sha256.New()

	fromJSON, _ := json.Marshal(from)
	toJSON, _ := json.Marshal(to)
	h.Write(fromJSON)
	h.Write([]byte{0})
	h.Write(toJSON)
	h.Write([]byte{0})

	// Include change kinds and entity IDs in the checksum
	for _, c := range diff.Changes {
		h.Write([]byte{byte(c.Kind)})
		h.Write([]byte(c.EntityId))
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

// DryRun computes what a migration would do without executing it.
// Returns the from/to versions, impact classification, change summaries,
// checksum, proposed phases, and the raw schema diff.
func (e *MigrationEngine) DryRun(
	ctx context.Context,
	collection string,
	newSchema *definition.Schema,
) (*DryRunResult, error) {
	entry, err := e.registry.GetRegistryEntry(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("get registry entry: %w", err)
	}

	oldSchema, err := e.registry.GetSchema(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("get current schema: %w", err)
	}

	diff, err := definition.Diff(oldSchema, newSchema)
	if err != nil {
		return nil, fmt.Errorf("compute diff: %w", err)
	}

	if len(diff.Changes) == 0 {
		return nil, ErrNoChanges
	}

	bump := definition.VersionImpact(diff)
	fromVersion := entry.ActiveVersion
	newVersion := bump.Apply(*fromVersion)

	impact := ClassifyImpact(diff)
	checksum := computeChecksum(oldSchema, newSchema, diff)
	summaries := BuildChangeSummaries(diff)

	phases := e.planPhases(impact, diff)

	return &DryRunResult{
		FromVersion: fromVersion.String(),
		ToVersion:   newVersion.String(),
		Impact:      impact,
		Changes:     summaries,
		Checksum:    checksum,
		Phases:      phases,
		Diff:        diff,
	}, nil
}

// planPhases determines the execution phases based on impact and diff content.
func (e *MigrationEngine) planPhases(impact ChangeImpact, diff *definition.SchemaDiff) []base.MigrationPhase {
	phases := []base.MigrationPhase{base.PhaseSchemaOnly}

	switch impact {
	case ImpactSafe:
		// Only schema registration needed
	case ImpactModerate:
		phases = append(phases, base.PhaseDDL)
	case ImpactBreaking:
		phases = append(phases, base.PhaseDDL, base.PhaseFull)
	}

	return phases
}

// Migrate executes a schema migration: diffs the current and new schemas,
// bumps the version, registers the new schema, and applies data migration
// if required. Returns the MigrationRecord with full lifecycle details.
func (e *MigrationEngine) Migrate(
	ctx context.Context,
	collection string,
	newSchema *definition.Schema,
) (*MigrationRecord, error) {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return nil, ErrMigrationInProgress
	}
	e.running = true
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
	}()

	startTime := time.Now()

	// --- Phase 1: Diff & Plan ---
	entry, err := e.registry.GetRegistryEntry(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("get registry entry: %w", err)
	}

	oldSchema, err := e.registry.GetSchema(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("get current schema: %w", err)
	}

	diff, err := definition.Diff(oldSchema, newSchema)
	if err != nil {
		return nil, fmt.Errorf("compute diff: %w", err)
	}

	if len(diff.Changes) == 0 {
		return nil, ErrNoChanges
	}

	bump := definition.VersionImpact(diff)
	fromVersion := entry.ActiveVersion
	newVersion := bump.Apply(*fromVersion)

	impact := ClassifyImpact(diff)
	checksum := computeChecksum(oldSchema, newSchema, diff)
	phases := e.planPhases(impact, diff)

	record := &MigrationRecord{
		ID:          fmt.Sprintf("%s-%s", collection, newVersion.String()),
		FromVersion: fromVersion.String(),
		ToVersion:   newVersion.String(),
		FromSchema:  oldSchema,
		ToSchema:    newSchema,
		Diff:        diff,
		Impact:      impact,
		Checksum:    checksum,
		Status:      StatusPending,
		Phases:      phases,
		CreatedAt:   startTime,
	}

	// --- Phase 2: Schema-only (register new version) ---
	record.Status = StatusRunning
	now := time.Now()
	record.StartedAt = &now

	if err := e.applySchemaPhase(ctx, collection, newVersion.String(), newSchema, diff); err != nil {
		return e.failRecord(record, err)
	}

	// --- Phase 3: DDL (data migration if required) ---
	if containsPhase(phases, base.PhaseDDL) {
		if err := e.applyDataPhase(ctx, collection, fromVersion.String(), newVersion.String()); err != nil {
			return e.failRecord(record, err)
		}
	}

	// --- Phase 4: Activate new version ---
	if _, err := e.registry.SetActiveVersion(ctx, collection, newVersion.String()); err != nil {
		return e.failRecord(record, fmt.Errorf("activate version: %w", err))
	}

	// --- Record success ---
	completeTime := time.Now()
	record.Status = StatusApplied
	record.CompletedAt = &completeTime

	e.mu.Lock()
	e.history = append(e.history, record)
	e.mu.Unlock()

	return record, nil
}

// applySchemaPhase registers the new schema version in the registry.
func (e *MigrationEngine) applySchemaPhase(
	ctx context.Context,
	collection, version string,
	schema *definition.Schema,
	diff *definition.SchemaDiff,
) error {
	// AddSchemaVersion provisions the new physical collection and registers
	// the schema in the registry. The physical name is derived automatically.
	if _, err := e.registry.AddSchemaVersion(ctx, collection, version, schema); err != nil {
		return fmt.Errorf("add schema version: %w", err)
	}
	return nil
}

// applyDataPhase copies and transforms documents from the old version
// to the new version using the registered transformer.
func (e *MigrationEngine) applyDataPhase(
	ctx context.Context,
	collection, fromVersion, toVersion string,
) error {
	key := collection + ":" + toVersion
	transformer, ok := e.transforms[key]
	if !ok {
		// No transformer registered — skip data migration (schema-only change)
		return nil
	}

	jobID, err := e.migrator.Migrate(ctx, collection, fromVersion, toVersion, transformer)
	if err != nil {
		return fmt.Errorf("data migration (job %s): %w", jobID, err)
	}

	// Optionally prune the old version's physical collection
	if _, err := e.registry.PruneVersion(ctx, collection, fromVersion); err != nil {
		// Prune failure is non-fatal — old data remains but version is inactive
		fmt.Printf("warning: prune version %s/%s: %v\n", collection, fromVersion, err)
	}

	return nil
}

// failRecord sets a MigrationRecord to failed status and appends it to history.
func (e *MigrationEngine) failRecord(record *MigrationRecord, err error) (*MigrationRecord, error) {
	record.Status = StatusFailed
	errStr := err.Error()
	record.Error = &errStr
	completeTime := time.Now()
	record.CompletedAt = &completeTime

	e.mu.Lock()
	e.history = append(e.history, record)
	e.mu.Unlock()

	return record, err
}

// Rollback reverts a migration by switching back to the previous schema version.
// It does NOT delete the target version — that is left to PruneVersion.
func (e *MigrationEngine) Rollback(
	ctx context.Context,
	collection string,
	targetVersion *string,
) (*MigrationRecord, error) {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return nil, ErrMigrationInProgress
	}
	e.running = true
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		e.running = false
		e.mu.Unlock()
	}()

	entry, err := e.registry.GetRegistryEntry(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("get registry entry: %w", err)
	}

	currentVersion := entry.ActiveVersion

	var rollbackTo *common.Version
	if targetVersion != nil {
		parsed, err := common.NewVersion(*targetVersion)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrInvalidVersion, *targetVersion)
		}
		rollbackTo = parsed
	} else {
		// Default: rollback to the previous version in history
		if len(entry.Versions) < 2 {
			return nil, fmt.Errorf("no previous version to rollback to")
		}
		// Find the version before current
		versions := make([]string, 0, len(entry.Versions))
		for v := range entry.Versions {
			versions = append(versions, v)
		}
		sort.Strings(versions)

		rollbackIdx := -1
		for i, v := range versions {
			if v == currentVersion.String() {
				rollbackIdx = i
				break
			}
		}
		if rollbackIdx <= 0 {
			return nil, fmt.Errorf("no previous version to rollback to")
		}
		parsed, err := common.NewVersion(versions[rollbackIdx-1])
		if err != nil {
			return nil, fmt.Errorf("parse rollback version: %w", err)
		}
		rollbackTo = parsed
	}

	startTime := nowPtr()
	record := &MigrationRecord{
		ID:          fmt.Sprintf("%s-rollback-%s", collection, rollbackTo.String()),
		FromVersion: currentVersion.String(),
		ToVersion:   rollbackTo.String(),
		Status:      StatusRunning,
		CreatedAt:  *startTime,
	}

	// Activate the target version
	if _, err := e.registry.SetActiveVersion(ctx, collection, rollbackTo.String()); err != nil {
		return e.failRecord(record, fmt.Errorf("activate rollback version: %w", err))
	}

	completeTime := nowPtr()
	record.Status = StatusApplied
	record.CompletedAt = completeTime
	record.RollbackOf = &collection

	e.mu.Lock()
	e.history = append(e.history, record)
	e.mu.Unlock()

	return record, nil
}

// History returns all migration records (applied, failed, rolled back).
func (e *MigrationEngine) History() []*MigrationRecord {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := make([]*MigrationRecord, len(e.history))
	copy(out, e.history)
	return out
}

// HistoryFor returns migration records for a specific collection prefix.
func (e *MigrationEngine) HistoryFor(collection string) []*MigrationRecord {
	e.mu.Lock()
	defer e.mu.Unlock()

	var out []*MigrationRecord
	for _, r := range e.history {
		if len(r.ID) >= len(collection) && r.ID[:len(collection)] == collection {
			out = append(out, r)
		}
	}
	return out
}

// Latest returns the most recent migration record, or nil if none exist.
func (e *MigrationEngine) Latest() *MigrationRecord {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.history) == 0 {
		return nil
	}
	return e.history[len(e.history)-1]
}

func containsPhase(phases []base.MigrationPhase, target base.MigrationPhase) bool {
	for _, p := range phases {
		if p == target {
			return true
		}
	}
	return false
}

func nowPtr() *time.Time {
	t := time.Now()
	return &t
}
