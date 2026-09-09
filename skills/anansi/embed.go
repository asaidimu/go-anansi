// Package anansi exposes the Anansi agent skill bundled with the CLI.
//
// The skill assets (SKILL.md, references/, evals/) are embedded into the
// binary so `anansi agents` can install them into a project (local, git root)
// or into the user's global agent skills directory without any copy step or
// network access. embed.go itself is not part of the embedded tree.
package anansi

import "embed"

// FS holds the skill assets installed by `anansi agents`.
//
//go:embed SKILL.md references evals
var FS embed.FS
