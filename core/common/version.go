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
	ErrHistoryFull       = errors.New("version history is full (max 1024 versions)")
	ErrInvalidAddress    = errors.New("invalid history address")
	ErrNotPrerelease     = errors.New("operation requires pre-release version")
	ErrAlreadyStable     = errors.New("version is already stable")
	ErrVersionNotFound   = errors.New("version not found in history")
)

type TagRank uint8

const (
	RankNone   TagRank = 0
	RankStable TagRank = 255
)

const MaxHistorySize = 1024 // 4 epochs × 256 versions

// VersionStrategy defines how versions are parsed and ranked.
type VersionStrategy struct {
	Name    string
	Tags    map[string]TagRank
	Default TagRank
}

// VersionRegistry manages version parsing with a specific strategy.
type VersionRegistry struct {
	strategy VersionStrategy
}

// NewVersionRegistry creates a new registry with the given strategy.
func NewVersionRegistry(strategy VersionStrategy) *VersionRegistry {
	return &VersionRegistry{strategy: strategy}
}

// Parse parses a version string using this registry's strategy.
func (r *VersionRegistry) Parse(version string) (*Version, error) {
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

		if rank, ok := r.strategy.Tags[tagName]; ok {
			v.TagType = rank
		} else {
			v.TagType = r.strategy.Default
		}

		if len(tagParts) > 1 {
			v.TagVersion, _ = strconv.Atoi(tagParts[1])
		}
	}

	return v, nil
}

// Strategy returns the registry's strategy (read-only).
func (r *VersionRegistry) Strategy() VersionStrategy {
	return r.strategy
}

// DefaultRegistry is the global default registry for backward compatibility.
var DefaultRegistry = NewVersionRegistry(VersionStrategy{
	Name: "standard",
	Tags: map[string]TagRank{
		"alpha": 10,
		"beta":  20,
		"rc":    30,
	},
	Default: RankNone,
})

// Legacy global state for backward compatibility
var (
	registryMu    sync.RWMutex
	configureOnce sync.Once
	isConfigured  bool
)

// ConfigureVersioning sets the global DefaultRegistry.
// Deprecated: Use NewVersionRegistry and pass registries explicitly.
// This is maintained for backward compatibility.
func ConfigureVersioning(strategy VersionStrategy) error {
	var err error

	configureOnce.Do(func() {
		registryMu.Lock()
		defer registryMu.Unlock()

		if isConfigured {
			err = ErrAlreadyConfigured
			return
		}

		DefaultRegistry = NewVersionRegistry(strategy)
		isConfigured = true
	})

	if err != nil {
		return err
	}

	registryMu.RLock()
	defer registryMu.RUnlock()
	return nil
}

// Version represents a semantic version.
type Version struct {
	Major      int
	Minor      int
	Patch      int
	TagType    TagRank
	TagVersion int
}

// MustNewVersion parses a version string using DefaultRegistry and panics on error.
func MustNewVersion(version string) *Version {
	v, err := NewVersion(version)
	if err != nil {
		panic(err)
	}
	return v
}

// NewVersion parses a version string using DefaultRegistry.
func NewVersion(version string) (*Version, error) {
	return DefaultRegistry.Parse(version)
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

// Compare compares two versions.
// Returns -1 if v < other, 0 if equal, 1 if v > other.
func (v *Version) Compare(other *Version) int {
	valA, valB := v.ToUint64(), other.ToUint64()
	if valA < valB {
		return -1
	}
	if valA > valB {
		return 1
	}
	return 0
}

// String returns the version as a string (uses DefaultRegistry for tag names).
func (v Version) String() string {
	base := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.TagType == RankStable {
		return base
	}

	strategy := DefaultRegistry.Strategy()
	tagName := "unknown"
	for name, rank := range strategy.Tags {
		if rank == v.TagType {
			tagName = name
			break
		}
	}
	return fmt.Sprintf("%s-%s.%d", base, tagName, v.TagVersion)
}

// StringWithRegistry returns the version string using a specific registry's tags.
func (v Version) StringWithRegistry(registry *VersionRegistry) string {
	base := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.TagType == RankStable {
		return base
	}

	strategy := registry.Strategy()
	tagName := "unknown"
	for name, rank := range strategy.Tags {
		if rank == v.TagType {
			tagName = name
			break
		}
	}
	return fmt.Sprintf("%s-%s.%d", base, tagName, v.TagVersion)
}

// BumpMajor returns a new version with major incremented and minor/patch reset.
func (v Version) BumpMajor() Version {
	return Version{Major: v.Major + 1, Minor: 0, Patch: 0, TagType: RankStable}
}

// BumpMinor returns a new version with minor incremented and patch reset.
func (v Version) BumpMinor() Version {
	return Version{Major: v.Major, Minor: v.Minor + 1, Patch: 0, TagType: RankStable}
}

// BumpPatch returns a new version with patch incremented.
func (v Version) BumpPatch() Version {
	return Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch + 1, TagType: RankStable}
}

// IsPrerelease returns true if this is a pre-release version.
func (v Version) IsPrerelease() bool {
	return v.TagType != RankStable
}

// SortVersions sorts versions in ascending order.
func SortVersions(versions []*Version) {
	slices.SortFunc(versions, func(a, b *Version) int {
		return a.Compare(b)
	})
}

// Latest returns the highest version from the slice.
func Latest(versions []*Version) *Version {
	if len(versions) == 0 {
		return nil
	}
	return slices.MaxFunc(versions, func(a, b *Version) int {
		return a.Compare(b)
	})
}

// MarshalJSON implements json.Marshaler.
func (v Version) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

// UnmarshalJSON implements json.Unmarshaler using DefaultRegistry.
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

// NewHistoryAddress creates a 10-bit address packed in uint16 (big-endian).
// Layout: [epoch 2 bits][index 8 bits][unused 6 bits]
//         EE IIIIIIII XXXXXX
func NewHistoryAddress(epoch, index uint8) uint16 {
	// Validate inputs
	epoch = epoch & 0x3    // 2 bits max
	// index is already 8 bits, no masking needed

	// Pack big-endian: epoch in bits 14-15, index in bits 6-13
	return (uint16(epoch) << 14) | (uint16(index) << 6)
}

// DecodeHistoryAddress extracts epoch and index from a packed address.
func DecodeHistoryAddress(addr uint16) (epoch, index uint8) {
	epoch = uint8((addr >> 14) & 0x3)
	index = uint8((addr >> 6) & 0xFF)
	return
}

// HistoryAddressToAbsolute converts a packed address to absolute index (0-1023).
func HistoryAddressToAbsolute(addr uint16) int {
	epoch, index := DecodeHistoryAddress(addr)
	return int(epoch)*256 + int(index)
}

// AbsoluteToHistoryAddress converts absolute index to packed address.
func AbsoluteToHistoryAddress(absolute int) uint16 {
	if absolute < 0 || absolute >= MaxHistorySize {
		return 0
	}
	epoch := uint8(absolute / 256)
	index := uint8(absolute % 256)
	return NewHistoryAddress(epoch, index)
}

// Versionable represents a versioned entity with history.
type Versionable struct {
	current  *Version
	history  []*Version
	registry *VersionRegistry
}

// NewVersionable creates a new versionable with optional history.
// History is provided as version strings that will be parsed using the registry.
func NewVersionable(registry *VersionRegistry, current string, history ...string) (*Versionable, error) {
	if registry == nil {
		return nil, errors.New("registry cannot be nil")
	}

	// Parse current version
	curr, err := registry.Parse(current)
	if err != nil {
		return nil, fmt.Errorf("invalid current version: %w", err)
	}

	// Parse history
	hist := make([]*Version, 0, len(history))
	for i, h := range history {
		v, err := registry.Parse(h)
		if err != nil {
			return nil, fmt.Errorf("invalid history version at index %d: %w", i, err)
		}
		hist = append(hist, v)
	}

	// Validate history size
	if len(hist) > MaxHistorySize {
		return nil, ErrHistoryFull
	}

	return &Versionable{
		current:  curr,
		history:  hist,
		registry: registry,
	}, nil
}

// Current returns the current version.
func (v *Versionable) Current() *Version {
	return v.current
}

// History returns a copy of the version history (read-only).
func (v *Versionable) History() []*Version {
	return slices.Clone(v.history)
}

// Registry returns the versionable's registry.
func (v *Versionable) Registry() *VersionRegistry {
	return v.registry
}

// TotalVersions returns the total number of versions (history + current).
func (v *Versionable) TotalVersions() int {
	return len(v.history) + 1
}

// Resolve converts a packed history address to a version.
func (v *Versionable) Resolve(addr uint16) (*Version, error) {
	absolute := HistoryAddressToAbsolute(addr)

	if absolute < 0 || absolute >= len(v.history) {
		return nil, ErrInvalidAddress
	}

	return v.history[absolute], nil
}

// Index finds a version in history and returns its packed address.
// Returns error if version is not found in history.
func (v *Versionable) Index(version *Version) (uint16, error) {
	if version == nil {
		return 0, ErrVersionNotFound
	}

	for i, h := range v.history {
		if h.Compare(version) == 0 {
			return AbsoluteToHistoryAddress(i), nil
		}
	}

	return 0, ErrVersionNotFound
}

// addToHistory appends the current version to history and updates current.
// This is internal and used by version operations.
func (v *Versionable) addToHistory(newCurrent *Version) error {
	if len(v.history) >= MaxHistorySize {
		return ErrHistoryFull
	}

	v.history = append(v.history, v.current)
	v.current = newCurrent
	return nil
}

// BumpMajor returns a new versionable with major version incremented.
func (v *Versionable) BumpMajor() (*Versionable, error) {
	newVer := v.current.BumpMajor()
	newVersionable := &Versionable{
		current:  &newVer,
		history:  slices.Clone(v.history),
		registry: v.registry,
	}
	if err := newVersionable.addToHistory(&newVer); err != nil {
		return nil, err
	}
	// Fix: current was added to history, restore correct state
	newVersionable.current = &newVer
	newVersionable.history = newVersionable.history[:len(newVersionable.history)-1]
	newVersionable.history = append(newVersionable.history, v.current)
	return newVersionable, nil
}

// BumpMinor returns a new versionable with minor version incremented.
func (v *Versionable) BumpMinor() (*Versionable, error) {
	newVer := v.current.BumpMinor()
	newVersionable := &Versionable{
		current:  &newVer,
		history:  append(slices.Clone(v.history), v.current),
		registry: v.registry,
	}
	return newVersionable, nil
}

// BumpPatch returns a new versionable with patch version incremented.
func (v *Versionable) BumpPatch() (*Versionable, error) {
	newVer := v.current.BumpPatch()
	newVersionable := &Versionable{
		current:  &newVer,
		history:  append(slices.Clone(v.history), v.current),
		registry: v.registry,
	}
	return newVersionable, nil
}

// Prerelease returns a new versionable with a pre-release version.
// Bumps minor version and adds the specified tag.
func (v *Versionable) Prerelease(tag string) (*Versionable, error) {
	// Validate tag exists in registry
	strategy := v.registry.Strategy()
	rank, ok := strategy.Tags[tag]
	if !ok {
		return nil, fmt.Errorf("unknown tag: %s", tag)
	}

	// Create new pre-release version (bump minor)
	base := v.current.BumpMinor()
	newVer := Version{
		Major:      base.Major,
		Minor:      base.Minor,
		Patch:      base.Patch,
		TagType:    rank,
		TagVersion: 1,
	}

	newVersionable := &Versionable{
		current:  &newVer,
		history:  append(slices.Clone(v.history), v.current),
		registry: v.registry,
	}
	return newVersionable, nil
}

// Increment returns a new versionable with pre-release version incremented.
// Only valid for pre-release versions.
func (v *Versionable) Increment() (*Versionable, error) {
	if !v.current.IsPrerelease() {
		return nil, ErrNotPrerelease
	}

	newVer := Version{
		Major:      v.current.Major,
		Minor:      v.current.Minor,
		Patch:      v.current.Patch,
		TagType:    v.current.TagType,
		TagVersion: v.current.TagVersion + 1,
	}

	newVersionable := &Versionable{
		current:  &newVer,
		history:  append(slices.Clone(v.history), v.current),
		registry: v.registry,
	}
	return newVersionable, nil
}

// Promote returns a new versionable promoted to a different pre-release tag.
// Tag version resets to 1.
func (v *Versionable) Promote(tag string) (*Versionable, error) {
	if !v.current.IsPrerelease() {
		return nil, ErrNotPrerelease
	}

	// Validate tag exists in registry
	strategy := v.registry.Strategy()
	rank, ok := strategy.Tags[tag]
	if !ok {
		return nil, fmt.Errorf("unknown tag: %s", tag)
	}

	newVer := Version{
		Major:      v.current.Major,
		Minor:      v.current.Minor,
		Patch:      v.current.Patch,
		TagType:    rank,
		TagVersion: 1,
	}

	newVersionable := &Versionable{
		current:  &newVer,
		history:  append(slices.Clone(v.history), v.current),
		registry: v.registry,
	}
	return newVersionable, nil
}

// Stabilize returns a new versionable with the version stabilized (removes pre-release tag).
func (v *Versionable) Stabilize() (*Versionable, error) {
	if !v.current.IsPrerelease() {
		return nil, ErrAlreadyStable
	}

	newVer := Version{
		Major:      v.current.Major,
		Minor:      v.current.Minor,
		Patch:      v.current.Patch,
		TagType:    RankStable,
		TagVersion: 0,
	}

	newVersionable := &Versionable{
		current:  &newVer,
		history:  append(slices.Clone(v.history), v.current),
		registry: v.registry,
	}
	return newVersionable, nil
}

// IsPrerelease returns true if the current version is a pre-release.
func (v *Versionable) IsPrerelease() bool {
	return v.current.IsPrerelease()
}

// VersionableJSON is used for JSON marshaling/unmarshaling.
type VersionableJSON struct {
	Current  string   `json:"current"`
	History  []string `json:"history"`
	Strategy string   `json:"strategy,omitempty"`
}

// MarshalJSON implements json.Marshaler.
func (v *Versionable) MarshalJSON() ([]byte, error) {
	history := make([]string, len(v.history))
	for i, h := range v.history {
		history[i] = h.StringWithRegistry(v.registry)
	}

	data := VersionableJSON{
		Current:  v.current.StringWithRegistry(v.registry),
		History:  history,
		Strategy: v.registry.Strategy().Name,
	}

	return json.Marshal(data)
}

// UnmarshalJSON implements json.Unmarshaler.
// Note: This uses DefaultRegistry. For custom registries, use UnmarshalVersionableJSON.
func (v *Versionable) UnmarshalJSON(data []byte) error {
	var vj VersionableJSON
	if err := json.Unmarshal(data, &vj); err != nil {
		return err
	}

	restored, err := NewVersionable(DefaultRegistry, vj.Current, vj.History...)
	if err != nil {
		return err
	}

	*v = *restored
	return nil
}

// UnmarshalVersionableJSON unmarshals a versionable using a specific registry.
func UnmarshalVersionableJSON(data []byte, registry *VersionRegistry) (*Versionable, error) {
	var vj VersionableJSON
	if err := json.Unmarshal(data, &vj); err != nil {
		return nil, err
	}

	return NewVersionable(registry, vj.Current, vj.History...)
}
