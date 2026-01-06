package common_test

import (
	"encoding/json"
	"testing"

	"github.com/asaidimu/go-anansi/v6/core/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Initialize the versioning strategy ONCE for the entire test suite
func init() {
	_ = common.ConfigureVersioning(common.VersionStrategy{
		Name: "test-suite-strategy",
		Tags: map[string]common.TagRank{
			"alpha": 10,
			"beta":  20,
			"rc":    30,
		},
		Default: common.RankNone,
	})
}

func TestConfigureVersioning(t *testing.T) {
	// Since init() already configured it, this SHOULD fail
	strategy := common.VersionStrategy{Name: "duplicate"}
	err := common.ConfigureVersioning(strategy)
	assert.ErrorIs(t, err, common.ErrAlreadyConfigured)
}

func TestNewVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected struct {
			Major   int
			TagType common.TagRank
			TagVer  int
		}
		wantErr bool
	}{
		{"1.2.3", struct {
			Major   int
			TagType common.TagRank
			TagVer  int
		}{1, common.RankStable, 0}, false},
		{"1.0.0-rc.2+build.123", struct {
			Major   int
			TagType common.TagRank
			TagVer  int
		}{1, 30, 2}, false}, // 30 is RankRC
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, err := common.NewVersion(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.Major, v.Major)
				assert.Equal(t, tt.expected.TagType, v.TagType)
				assert.Equal(t, tt.expected.TagVer, v.TagVersion)
			}
		})
	}
}

func TestBitfieldConversion(t *testing.T) {
	original := &common.Version{
		Major:      10,
		Minor:      25,
		Patch:      500,
		TagType:    30, // RankRC
		TagVersion: 3,
	}

	packed := original.ToUint64()
	assert.NotZero(t, packed)

	hydrated := common.NewVersionFromUint64(packed)
	assert.Equal(t, original, hydrated, "Hydrated version should match original")
}

func TestComparison(t *testing.T) {
	// 1.0.0-rc.1 < 1.0.0
	v1, _ := common.NewVersion("1.0.0-rc.1")
	v2, _ := common.NewVersion("1.0.0")
	assert.Equal(t, -1, v1.Compare(v2))

	// Higher Major version
	v3, _ := common.NewVersion("2.0.0")
	assert.Equal(t, 1, v3.Compare(v2))
}

func TestSorting(t *testing.T) {
	// Clearer test: rc (30) should be higher than alpha (10) but lower than stable (255)
	raw := []string{"1.2.3", "1.0.0", "1.2.3-rc.1", "2.0.0", "1.1.0"}
	versions := make([]*common.Version, len(raw))
	for i, s := range raw {
		v, err := common.NewVersion(s)
		require.NoError(t, err)
		versions[i] = v
	}

	common.SortVersions(versions)

	expectedOrder := []string{
		"1.0.0",
		"1.1.0",
		"1.2.3-rc.1",
		"1.2.3",
		"2.0.0",
	}

	actualStrings := make([]string, len(versions))
	for i, v := range versions {
		actualStrings[i] = v.String()
	}

	assert.Equal(t, expectedOrder, actualStrings)
}
func TestJSON(t *testing.T) {
	v, err := common.NewVersion("1.5.0-rc.2")
	require.NoError(t, err)

	data, err := json.Marshal(v)
	assert.NoError(t, err)
	assert.JSONEq(t, `"1.5.0-rc.2"`, string(data))

	var v2 common.Version
	err = json.Unmarshal(data, &v2)
	assert.NoError(t, err)
	assert.Zero(t, v.Compare(&v2), "Unmarshaled version should equal original")
}

func TestBumping(t *testing.T) {
	v, _ := common.NewVersion("1.2.3-rc.1")

	assert.Equal(t, "2.0.0", v.BumpMajor().String())
	assert.Equal(t, "1.3.0", v.BumpMinor().String())
	assert.Equal(t, "1.2.4", v.BumpPatch().String())

	// Ensure immutability
	assert.Equal(t, "1.2.3-rc.1", v.String())
}
