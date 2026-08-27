#!/usr/bin/env bash
# Copy plugin metadata to the repository root. Obsidian's community directory
# reads manifest.json at HEAD of the default branch, not from plugin/.
# Also copy the three sideload files into the dashboard /assets tree so a
# running server can serve them without a git clone.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cp "$ROOT/plugin/manifest.json" "$ROOT/manifest.json"
cp "$ROOT/plugin/versions.json" "$ROOT/versions.json"
DEST="$ROOT/internal/web/static/assets/obsidian"
mkdir -p "$DEST"
cp "$ROOT/plugin/main.js" "$ROOT/plugin/manifest.json" "$ROOT/plugin/styles.css" "$DEST/"
echo "Copied plugin/manifest.json and plugin/versions.json to the repository root."
echo "Copied plugin files to internal/web/static/assets/obsidian/ (served at /assets/obsidian/)."
