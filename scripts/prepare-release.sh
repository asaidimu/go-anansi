#!/usr/bin/env bash
# Stamp the release version into the npm manifests and rebuild dist so the
# @semantic-release/npm plugin publishes a fully-versioned package.
#
# Invoked by semantic-release: scripts/prepare-release.sh <version>
set -euo pipefail

VERSION="${1:?usage: prepare-release.sh <version>}"
cd "$(dirname "$0")/../packages/anansi"

bun -e "
import { readFileSync, writeFileSync } from 'node:fs';
const v = '${VERSION}';
const f = 'package.json';
const j = JSON.parse(readFileSync(f, 'utf8'));
j.version = v;
writeFileSync(f, JSON.stringify(j, null, 2) + '\n');
console.log('stamped', f, '→', v);
"
# dist/package.json is derived from package.json at build time (postbuild),
# so no second manifest needs stamping.

# Fresh build so dist/package.json carries the version.
bun run clean || true
bun install --frozen-lockfile
bun run build
