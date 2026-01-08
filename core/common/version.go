package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
)

var (
	ErrAlreadyConfigured = errors.New("versioning strategy has already been configured")
)

type TagRank uint8

const (
	RankNone   TagRank = 0
	RankStable TagRank = 255
)

type VersionStrategy struct {
	Name    string
	Tags    map[string]TagRank
	Default TagRank
}

var (
	registryMu      sync.RWMutex
	configureOnce   sync.Once
	isConfigured    bool
	currentStrategy = &VersionStrategy{
		Name: "standard",
		Tags: map[string]TagRank{
			"alpha": 10,
			"beta":  20,
			"rc":    30,
		},
		Default: RankNone,
	}
)

// ConfigureVersioning sets the global behavior for versioning.
// It must be called once at application startup.
func ConfigureVersioning(strategy VersionStrategy) error {
	var err error

	configureOnce.Do(func() {
		registryMu.Lock()
		defer registryMu.Unlock()

		if isConfigured {
			err = ErrAlreadyConfigured
			return
		}

		currentStrategy = &strategy
		isConfigured = true
	})

	if err != nil {
		return err
	}

	// Verify if this specific call was the one that configured the factory
	registryMu.RLock()
	defer registryMu.RUnlock()
	if currentStrategy != &strategy {
		return ErrAlreadyConfigured
	}

	return nil
}

func getStrategy() *VersionStrategy {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return currentStrategy
}

type Version struct {
	Major      int
	Minor      int
	Patch      int
	TagType    TagRank
	TagVersion int
}


func MustNewVersion(version string) (*Version) {
	v, err := NewVersion(version)
	if err != nil {
		panic(err)
	}
	return v
}

// NewVersion parses a string using the immutable global strategy.
func NewVersion(version string) (*Version, error) {
	v := &Version{TagType: RankStable}
	clean := strings.TrimSpace(version)
	clean = strings.TrimPrefix(clean, "v")
	clean = strings.TrimPrefix(clean, "V")

	mainParts := strings.SplitN(clean, "-", 2)
	vParts := strings.Split(mainParts[0], ".")
	if len(vParts) != 3 {
		return nil, fmt.Errorf("invalid version format: %q", version)
	}

	var err error
	if v.Major, err = strconv.Atoi(vParts[0]); err != nil {
		return nil, fmt.Errorf("invalid major: %w", err)
	}
	if v.Minor, err = strconv.Atoi(vParts[1]); err != nil {
		return nil, fmt.Errorf("invalid minor: %w", err)
	}
	if v.Patch, err = strconv.Atoi(vParts[2]); err != nil {
		return nil, fmt.Errorf("invalid patch: %w", err)
	}

	if len(mainParts) > 1 {
		tagBody := mainParts[1]
		if idx := strings.Index(tagBody, "+"); idx != -1 {
			tagBody = tagBody[:idx]
		}
		tagParts := strings.Split(tagBody, ".")
		tagName := strings.ToLower(tagParts[0])

		strategy := getStrategy()
		if rank, ok := strategy.Tags[tagName]; ok {
			v.TagType = rank
		} else {
			v.TagType = strategy.Default
		}

		if len(tagParts) > 1 {
			v.TagVersion, _ = strconv.Atoi(tagParts[1])
		}
	}

	return v, nil
}

// NewVersionFromUint64 hydrates a Version struct from a packed 64-bit integer.
func NewVersionFromUint64(val uint64) *Version {
	return &Version{
		Major:      int((val >> 48) & 0xFFFF),
		Minor:      int((val >> 32) & 0xFFFF),
		Patch:      int((val >> 16) & 0xFFFF),
		TagType:    TagRank((val >> 8) & 0xFF),
		TagVersion: int(val & 0xFF),
	}
}

// ToUint64 packs the version into a 64-bit integer.
// [Major 16][Minor 16][Patch 16][TagRank 8][TagVersion 8]
func (v *Version) ToUint64() uint64 {
	if v == nil {
		return 0
	}
	var res uint64
	res |= (uint64(v.Major) & 0xFFFF) << 48
	res |= (uint64(v.Minor) & 0xFFFF) << 32
	res |= (uint64(v.Patch) & 0xFFFF) << 16
	res |= (uint64(v.TagType) & 0xFF) << 8
	res |= (uint64(v.TagVersion) & 0xFF)
	return res
}

// --- 4. LOGIC & UTILITIES ---


func (v *Version) Compare(other *Version) int {
	valA, valB := v.ToUint64(), other.ToUint64()
	if valA < valB { return -1 }
	if valA > valB { return 1 }
	return 0
}

func (v Version) String() string {
	base := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.TagType == RankStable {
		return base
	}

	strategy := getStrategy()
	tagName := "unknown"
	for name, rank := range strategy.Tags {
		if rank == v.TagType {
			tagName = name
			break
		}
	}
	return fmt.Sprintf("%s-%s.%d", base, tagName, v.TagVersion)
}

func (v Version) BumpMajor() Version {
	return Version{Major: v.Major + 1, Minor: 0, Patch: 0, TagType: RankStable}
}

func (v Version) BumpMinor() Version {
	return Version{Major: v.Major, Minor: v.Minor + 1, Patch: 0, TagType: RankStable}
}

func (v Version) BumpPatch() Version {
	return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1, TagType: RankStable}
}

func SortVersions(versions []*Version) {
	slices.SortFunc(versions, func(a, b *Version) int {
		return a.Compare(b)
	})
}

func Latest(versions []*Version) *Version {
	if len(versions) == 0 { return nil }
	return slices.MaxFunc(versions, func(a, b *Version) int {
		return a.Compare(b)
	})
}

func (v *Version) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

func (v *Version) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := NewVersion(s)
	if err != nil {
		return err
	}
	*v = *parsed
	return nil
}
