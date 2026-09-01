package migration

import (
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/persistence/base"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

func makeField(name string, required bool, ftype definition.FieldType) definition.Field {
	return definition.Field{
		Name:     definition.FieldName(name),
		Required: required,
		FieldProperties: definition.FieldProperties{
			Type: ftype,
		},
	}
}

func makeIndex(name string, fields []definition.FieldName, unique bool) definition.Index {
	return definition.Index{
		Name:   name,
		Type:   definition.IndexTypeNormal,
		Fields: fields,
		Unique: unique,
	}
}

func makeSchema(name string, fields map[definition.FieldId]definition.Field) *definition.Schema {
	return &definition.Schema{
		BaseSchema: definition.BaseSchema{
			Name:   name,
			Fields: fields,
		},
	}
}

// --- ClassifyImpact ---

func TestClassifyImpact_NoChanges(t *testing.T) {
	diff := &definition.SchemaDiff{}
	impact := ClassifyImpact(diff)
	if impact != ImpactSafe {
		t.Errorf("expected ImpactSafe for empty diff, got %v", impact)
	}
}

func TestClassifyImpact_AddOptionalField(t *testing.T) {
	diff := &definition.SchemaDiff{
		Changes: []definition.SemanticChange{
			{
				Kind:     definition.FieldAdded,
				EntityId: "bio",
				Forward: []definition.Operation{
					{Type: definition.OpAdd, Value: makeField("bio", false, definition.FieldTypeString)},
				},
			},
		},
	}
	impact := ClassifyImpact(diff)
	if impact != ImpactModerate {
		t.Errorf("expected ImpactModerate for optional field addition, got %v", impact)
	}
}

func TestClassifyImpact_AddRequiredField(t *testing.T) {
	diff := &definition.SchemaDiff{
		Changes: []definition.SemanticChange{
			{
				Kind:     definition.FieldAdded,
				EntityId: "email",
				Forward: []definition.Operation{
					{Type: definition.OpAdd, Value: makeField("email", true, definition.FieldTypeString)},
				},
			},
		},
	}
	impact := ClassifyImpact(diff)
	if impact != ImpactBreaking {
		t.Errorf("expected ImpactBreaking for required field addition, got %v", impact)
	}
}

func TestClassifyImpact_RemoveField(t *testing.T) {
	diff := &definition.SchemaDiff{
		Changes: []definition.SemanticChange{
			{
				Kind:     definition.FieldRemoved,
				EntityId: "old_field",
			},
		},
	}
	impact := ClassifyImpact(diff)
	if impact != ImpactBreaking {
		t.Errorf("expected ImpactBreaking for field removal, got %v", impact)
	}
}

func TestClassifyImpact_TypeChange(t *testing.T) {
	diff := &definition.SchemaDiff{
		Changes: []definition.SemanticChange{
			{
				Kind:     definition.FieldModified,
				EntityId: "age",
				Forward: []definition.Operation{
					{
						Type: definition.OpSet,
						Path: definition.Path{
							Segments: []definition.PathSegment{
								{Type: definition.PathEntity, Key: "age"},
								{Type: definition.PathType},
							},
						},
						Value: definition.FieldTypeInteger,
					},
				},
			},
		},
	}
	impact := ClassifyImpact(diff)
	if impact != ImpactBreaking {
		t.Errorf("expected ImpactBreaking for type change, got %v", impact)
	}
}

func TestClassifyImpact_IndexAdded(t *testing.T) {
	diff := &definition.SchemaDiff{
		Changes: []definition.SemanticChange{
			{
				Kind:     definition.IndexAdded,
				EntityId: "idx_email",
			},
		},
	}
	impact := ClassifyImpact(diff)
	if impact != ImpactModerate {
		t.Errorf("expected ImpactModerate for index addition, got %v", impact)
	}
}

func TestClassifyImpact_MixedChanges(t *testing.T) {
	diff := &definition.SchemaDiff{
		Changes: []definition.SemanticChange{
			{
				Kind:     definition.FieldAdded,
				EntityId: "bio",
				Forward: []definition.Operation{
					{Type: definition.OpAdd, Value: makeField("bio", false, definition.FieldTypeString)},
				},
			},
			{
				Kind:     definition.IndexAdded,
				EntityId: "idx_bio",
			},
		},
	}
	impact := ClassifyImpact(diff)
	if impact != ImpactModerate {
		t.Errorf("expected ImpactModerate, got %v", impact)
	}
}

// --- BuildChangeSummaries ---

func TestBuildChangeSummaries(t *testing.T) {
	diff := &definition.SchemaDiff{
		Changes: []definition.SemanticChange{
			{
				Kind:     definition.FieldAdded,
				EntityId: "bio",
				Forward:  []definition.Operation{{Type: definition.OpAdd}},
			},
			{
				Kind:     definition.FieldRemoved,
				EntityId: "legacy",
				Forward:  []definition.Operation{{Type: definition.OpRemove}},
			},
			{
				Kind:     definition.IndexAdded,
				EntityId: "idx_bio",
				Forward:  []definition.Operation{{Type: definition.OpAdd}},
			},
		},
	}
	summaries := BuildChangeSummaries(diff)
	if len(summaries) != 3 {
		t.Fatalf("expected 3 summaries, got %d", len(summaries))
	}

	if summaries[0].Kind != "field_added" || summaries[0].Entity != "bio" {
		t.Errorf("unexpected summary[0]: %+v", summaries[0])
	}
	if summaries[1].Kind != "field_removed" || summaries[1].Entity != "legacy" {
		t.Errorf("unexpected summary[1]: %+v", summaries[1])
	}
	if summaries[2].Kind != "index_added" || summaries[2].Entity != "idx_bio" {
		t.Errorf("unexpected summary[2]: %+v", summaries[2])
	}
}

func TestBuildChangeSummaries_RequiresData(t *testing.T) {
	diff := &definition.SchemaDiff{
		Changes: []definition.SemanticChange{
			{
				Kind:     definition.FieldModified,
				EntityId: "age",
				Forward: []definition.Operation{
					{
						Type: definition.OpSet,
						Path: definition.Path{
							Segments: []definition.PathSegment{
								{Type: definition.PathEntity, Key: "age"},
								{Type: definition.PathType},
							},
						},
					},
				},
			},
		},
	}
	summaries := BuildChangeSummaries(diff)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if !summaries[0].RequiresData {
		t.Error("expected RequiresData=true for type change")
	}
}

// --- Checksum ---

func TestComputeChecksum_Deterministic(t *testing.T) {
	s1 := makeSchema("test", map[definition.FieldId]definition.Field{
		"f1": makeField("name", false, definition.FieldTypeString),
	})
	s2 := makeSchema("test", map[definition.FieldId]definition.Field{
		"f1": makeField("name", false, definition.FieldTypeString),
		"f2": makeField("email", true, definition.FieldTypeString),
	})

	diff, _ := definition.Diff(s1, s2)

	c1 := computeChecksum(s1, s2, diff)
	c2 := computeChecksum(s1, s2, diff)
	if c1 != c2 {
		t.Errorf("checksums not deterministic: %s != %s", c1, c2)
	}
	if len(c1) != 64 { // SHA-256 hex string
		t.Errorf("unexpected checksum length: %d", len(c1))
	}
}

func TestComputeChecksum_DifferentSchemas(t *testing.T) {
	s1 := makeSchema("a", map[definition.FieldId]definition.Field{
		"f1": makeField("name", false, definition.FieldTypeString),
	})
	s2 := makeSchema("a", map[definition.FieldId]definition.Field{
		"f1": makeField("name", false, definition.FieldTypeString),
		"f2": makeField("email", true, definition.FieldTypeString),
	})
	s3 := makeSchema("a", map[definition.FieldId]definition.Field{
		"f1": makeField("name", false, definition.FieldTypeString),
		"f2": makeField("phone", false, definition.FieldTypeString),
	})

	diff12, _ := definition.Diff(s1, s2)
	diff13, _ := definition.Diff(s1, s3)

	c12 := computeChecksum(s1, s2, diff12)
	c13 := computeChecksum(s1, s3, diff13)
	if c12 == c13 {
		t.Error("different schemas should produce different checksums")
	}
}

// --- Phase Planning ---

func TestPlanPhases_SafeChange(t *testing.T) {
	e := &MigrationEngine{transforms: make(map[string]Transformer)}
	diff := &definition.SchemaDiff{
		Changes: []definition.SemanticChange{
			{Kind: definition.IndexAdded, EntityId: "idx_new"},
		},
	}
	phases := e.planPhases(ImpactSafe, diff)
	if len(phases) != 1 || phases[0] != base.PhaseSchemaOnly {
		t.Errorf("expected [PhaseSchemaOnly], got %v", phases)
	}
}

func TestPlanPhases_ModerateChange(t *testing.T) {
	e := &MigrationEngine{transforms: make(map[string]Transformer)}
	diff := &definition.SchemaDiff{}
	phases := e.planPhases(ImpactModerate, diff)
	if len(phases) != 2 || phases[0] != base.PhaseSchemaOnly || phases[1] != base.PhaseDDL {
		t.Errorf("expected [PhaseSchemaOnly PhaseDDL], got %v", phases)
	}
}

func TestPlanPhases_BreakingChange(t *testing.T) {
	e := &MigrationEngine{transforms: make(map[string]Transformer)}
	diff := &definition.SchemaDiff{}
	phases := e.planPhases(ImpactBreaking, diff)
	if len(phases) != 3 {
		t.Fatalf("expected 3 phases, got %d", len(phases))
	}
	if phases[0] != base.PhaseSchemaOnly || phases[1] != base.PhaseDDL || phases[2] != base.PhaseFull {
		t.Errorf("expected [SchemaOnly DDL Full], got %v", phases)
	}
}

// --- History ---

func TestHistory_Empty(t *testing.T) {
	e := &MigrationEngine{transforms: make(map[string]Transformer)}
	if len(e.History()) != 0 {
		t.Error("expected empty history")
	}
	if e.Latest() != nil {
		t.Error("expected nil latest")
	}
}

func TestHistoryFor(t *testing.T) {
	e := &MigrationEngine{transforms: make(map[string]Transformer)}
	e.history = []*MigrationRecord{
		{ID: "users-2.0.0"},
		{ID: "posts-1.1.0"},
		{ID: "users-2.1.0"},
	}
	users := e.HistoryFor("users")
	if len(users) != 2 {
		t.Errorf("expected 2 users migrations, got %d", len(users))
	}
}

// --- SummarizeChange ---

func TestSummarizeChange(t *testing.T) {
	tests := []struct {
		kind     definition.ChangeKind
		entity   string
		expected string
	}{
		{definition.FieldAdded, "email", `add field "email"`},
		{definition.FieldRemoved, "old", `remove field "old"`},
		{definition.FieldModified, "name", `modify field "name"`},
		{definition.IndexAdded, "idx_email", `add index "idx_email"`},
		{definition.IndexRemoved, "idx_old", `remove index "idx_old"`},
		{definition.IndexModified, "idx_name", `modify index "idx_name"`},
		{definition.ConstraintAdded, "ck_age", `add constraint "ck_age"`},
		{definition.ConstraintRemoved, "ck_old", `remove constraint "ck_old"`},
		{definition.ConstraintModified, "ck_check", `modify constraint "ck_check"`},
		{definition.SchemaAdded, "address", `add nested schema "address"`},
		{definition.SchemaRemoved, "legacy", `remove nested schema "legacy"`},
		{definition.SchemaModified, "profile", `modify nested schema "profile"`},
	}

	for _, tt := range tests {
		c := definition.SemanticChange{Kind: tt.kind, EntityId: tt.entity}
		got := summarizeChange(c)
		if got != tt.expected {
			t.Errorf("summarizeChange(%v, %q) = %q, want %q", tt.kind, tt.entity, got, tt.expected)
		}
	}
}

// --- ChangeImpact String ---

func TestChangeImpact_String(t *testing.T) {
	tests := []struct {
		impact   ChangeImpact
		expected string
	}{
		{ImpactNone, "none"},
		{ImpactSafe, "safe"},
		{ImpactModerate, "moderate"},
		{ImpactBreaking, "breaking"},
	}
	for _, tt := range tests {
		if got := tt.impact.String(); got != tt.expected {
			t.Errorf("ChangeImpact(%d).String() = %q, want %q", tt.impact, got, tt.expected)
		}
	}
}

// --- Phase Planning Integration ---

func TestPlanPhases_ConsistencyWithImpact(t *testing.T) {
	e := &MigrationEngine{transforms: make(map[string]Transformer)}

	cases := []struct {
		impact      ChangeImpact
		expectedLen int
		firstPhase  base.MigrationPhase
	}{
		{ImpactSafe, 1, base.PhaseSchemaOnly},
		{ImpactModerate, 2, base.PhaseSchemaOnly},
		{ImpactBreaking, 3, base.PhaseSchemaOnly},
	}

	for _, tc := range cases {
		phases := e.planPhases(tc.impact, &definition.SchemaDiff{})
		if len(phases) != tc.expectedLen {
			t.Errorf("impact %v: expected %d phases, got %d", tc.impact, tc.expectedLen, len(phases))
		}
		if phases[0] != tc.firstPhase {
			t.Errorf("impact %v: first phase should be %v, got %v", tc.impact, tc.firstPhase, phases[0])
		}
	}
}

// --- DryRunResult Structure ---

func TestDryRunResult_Fields(t *testing.T) {
	result := &DryRunResult{
		FromVersion: "1.0.0",
		ToVersion:   "1.1.0",
		Impact:      ImpactSafe,
		Changes: []ChangeSummary{
			{Kind: "field_added", Entity: "bio", Detail: `add field "bio"`, RequiresData: false},
		},
		Checksum: "abc123",
		Phases:   []base.MigrationPhase{base.PhaseSchemaOnly},
	}

	if result.FromVersion != "1.0.0" {
		t.Error("unexpected FromVersion")
	}
	if result.ToVersion != "1.1.0" {
		t.Error("unexpected ToVersion")
	}
	if result.Impact != ImpactSafe {
		t.Error("unexpected Impact")
	}
	if len(result.Changes) != 1 {
		t.Error("unexpected Changes length")
	}
	if result.Checksum != "abc123" {
		t.Error("unexpected Checksum")
	}
}
