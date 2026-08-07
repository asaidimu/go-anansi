package schemagen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstallSkill(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".agents", "skills", "anansi")
	require.NoError(t, InstallSkill(target, false))

	// Core assets land on disk.
	require.FileExists(t, filepath.Join(target, "SKILL.md"))
	require.FileExists(t, filepath.Join(target, "evals", "evals.json"))
	refs, err := os.ReadDir(filepath.Join(target, "references"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(refs), 5, "all reference docs must be installed")

	// Go sources are never installed.
	require.NoFileExists(t, filepath.Join(target, "embed.go"))
}

func TestInstallSkill_DryRun(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".agents", "skills", "anansi")
	require.NoError(t, InstallSkill(target, true))
	require.NoDirExists(t, filepath.Join(target, "references"))
	require.NoFileExists(t, filepath.Join(target, "SKILL.md"))
}

func TestInstallSkill_Overwrites(t *testing.T) {
	target := filepath.Join(t.TempDir(), ".agents", "skills", "anansi")
	require.NoError(t, os.MkdirAll(filepath.Join(target, "references"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("stale"), 0644))

	require.NoError(t, InstallSkill(target, false))

	data, err := os.ReadFile(filepath.Join(target, "SKILL.md"))
	require.NoError(t, err)
	require.NotEqual(t, "stale", string(data), "install must refresh existing skill files")
}

func TestFindGitRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "a", "b"), 0755))

	got, err := FindGitRoot(filepath.Join(root, "a", "b"))
	require.NoError(t, err)
	require.Equal(t, root, got)

	// No .git marker anywhere above -> empty root.
	nowhere := filepath.Join(t.TempDir(), "a", "b")
	require.NoError(t, os.MkdirAll(nowhere, 0755))
	got, err = FindGitRoot(nowhere)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestDefaultSkillDir_UnderGitRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".git"), 0755))

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(oldWd)
	require.NoError(t, os.Chdir(root))

	require.Equal(t, filepath.Join(root, ".agents", "skills", "anansi"), DefaultSkillDir())
}

func TestGlobalSkillDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := GlobalSkillDir()
	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".agents", "skills", "anansi"), got)
}

func TestScaffoldAgentsMD_NonEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new-project")
	require.NoError(t, os.MkdirAll(dir, 0755))
	written, err := writeScaffoldFile(filepath.Join(dir, "AGENTS.md"), []byte(scaffoldAgentsMD), false)
	require.NoError(t, err)
	require.True(t, written)

	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	require.True(t, strings.Contains(string(data), "anansi"), "scaffold AGENTS.md must reference the anansi CLI")
	require.True(t, strings.Contains(string(data), "go test ./..."), "scaffold AGENTS.md must document the test command")
}
