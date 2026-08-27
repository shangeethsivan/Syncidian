#!/usr/bin/env bash
# Copy plugin metadata to the repository root. Obsidian's community directory
# reads manifest.json at HEAD of the default branch, not from plugin/.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cp "$ROOT/plugin/manifest.json" "$ROOT/manifest.json"
cp "$ROOT/plugin/versions.json" "$ROOT/versions.json"
echo "Copied plugin/manifest.json and plugin/versions.json to the repository root."
