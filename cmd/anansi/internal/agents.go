package schemagen

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	ananskill "github.com/asaidimu/go-anansi/v8/skills/anansi"
)

// DefaultSkillDir returns the local install target: the current project's git
// root (falling back to the working directory when no repo is found) plus the
// agent skills path. Local installs live under .agents/skills so project-scoped
// agent tools pick the skill up automatically.
func DefaultSkillDir() string {
	root, err := FindGitRoot(".")
	if err != nil || root == "" {
		root = "."
	}
	return filepath.Join(root, ".agents", "skills", "anansi")
}

// GlobalSkillDir returns the user-global install target.
func GlobalSkillDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".agents", "skills", "anansi"), nil
}

// FindGitRoot walks up from start looking for a directory containing .git and
// returns its absolute path. It returns an empty string when no git root is
// found.
func FindGitRoot(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if fi, err := os.Stat(filepath.Join(abs, ".git")); err == nil && fi.IsDir() {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", nil
		}
		abs = parent
	}
}

// InstallSkill copies the bundled skill assets into target (e.g.
// <root>/.agents/skills/anansi), overwriting any existing files so installs
// double as refreshes. Go sources such as embed.go are never part of the
// embedded tree and are not installed.
func InstallSkill(target string, dryRun bool) error {
	var files []string
	err := fs.WalkDir(ananskill.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk skill assets: %w", err)
	}
	sort.Strings(files)

	for _, f := range files {
		dest := filepath.Join(target, filepath.FromSlash(f))
		if dryRun {
			fmt.Printf("would install: %s\n", dest)
			continue
		}
		data, err := ananskill.FS.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", f, err)
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return fmt.Errorf("create dir %s: %w", filepath.Dir(dest), err)
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
	}

	if dryRun {
		fmt.Printf("would install skill (%d files) to: %s\n", len(files), target)
		return nil
	}
	fmt.Printf("installed skill (%d files): %s\n", len(files), target)
	return nil
}
