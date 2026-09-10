package registry

import (
	"regexp"
	"testing"

	"github.com/asaidimu/go-anansi/v8/core/common"
	"github.com/asaidimu/go-anansi/v8/core/schema/definition"
)

func testSchema(name, version string) *definition.Schema {
	return &definition.Schema{
		BaseSchema: definition.BaseSchema{Name: name},
		Version:    common.MustNewVersion(version),
	}
}

// The reported bug: distinct collections/views sharing a name prefix and
// version truncated to the SAME physical table, so CTAS silently reused a
// stale table. They must now resolve to distinct physical names.
func TestGeneratePhysicalNameNoTruncationCollision(t *testing.T) {
	a, err := generatePhysicalName(testSchema("e2e_view_snapshot_alpha_run_aaa111", "1.0.0"))
	if err != nil {
		t.Fatalf("name A: %v", err)
	}
	b, err := generatePhysicalName(testSchema("e2e_view_snapshot_alpha_run_bbb222", "1.0.0"))
	if err != nil {
		t.Fatalf("name B: %v", err)
	}
	if a == b {
		t.Fatalf("collision: distinct logical names mapped to %q", a)
	}
	if len(a) > 24 || len(b) > 24 {
		t.Fatalf("exceeds 24-char limit: %q, %q", a, b)
	}
	t.Logf("distinct physical names: %q vs %q", a, b)
}

// Names that sanitize identically must still diverge: the hash covers the
// raw logical identity, not the sanitized form.
func TestGeneratePhysicalNameSanitizationCollision(t *testing.T) {
	a, err := generatePhysicalName(testSchema("my-very-long-collection-name-here", "1.0.0"))
	if err != nil {
		t.Fatalf("dash name: %v", err)
	}
	b, err := generatePhysicalName(testSchema("my_very_long_collection_name_here", "1.0.0"))
	if err != nil {
		t.Fatalf("underscore name: %v", err)
	}
	if a == b {
		t.Fatalf("collision: %q vs %q", a, b)
	}
}

func TestGeneratePhysicalNameDeterministic(t *testing.T) {
	sc := testSchema("e2e_view_snapshot_alpha_run_aaa111", "1.0.0")
	first, err := generatePhysicalName(sc)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		got, err := generatePhysicalName(sc)
		if err != nil {
			t.Fatal(err)
		}
		if got != first {
			t.Fatalf("nondeterministic: %q vs %q", first, got)
		}
	}
}

// Short names keep the legacy name_version form byte-for-byte so existing
// physical tables keep resolving.
func TestGeneratePhysicalNameLegacyShortNames(t *testing.T) {
	cases := map[string]string{
		"users":  "users_1_0_0",
		"notes":  "notes_1_0_0",
		"orders": "orders_2_3_4",
	}
	for name, want := range cases {
		version := "1.0.0"
		if name == "orders" {
			version = "2.3.4"
		}
		got, err := generatePhysicalName(testSchema(name, version))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != want {
			t.Errorf("%s: got %q, want legacy %q", name, got, want)
		}
	}
}

func TestGeneratePhysicalNameShape(t *testing.T) {
	valid := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	names := []string{
		"users",
		"e2e_view_snapshot_alpha_run_aaa111",
		"9lives开始", // leading digit + non-ASCII: t_ prefix, sanitized
		"My-Mixed_CASE.Collection!",
	}
	for _, name := range names {
		got, err := generatePhysicalName(testSchema(name, "1.0.0"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(got) > 24 {
			t.Errorf("%s: %q exceeds 24 chars", name, got)
		}
		if !valid.MatchString(got) {
			t.Errorf("%s: %q is not a safe identifier", name, got)
		}
	}
}

func TestGeneratePhysicalNameErrors(t *testing.T) {
	if _, err := generatePhysicalName(testSchema("", "1.0.0")); err == nil {
		t.Error("empty name: expected error")
	}
	noVersion := &definition.Schema{
		BaseSchema: definition.BaseSchema{Name: "users"},
	}
	if _, err := generatePhysicalName(noVersion); err == nil {
		t.Error("nil version: expected error")
	}
}
