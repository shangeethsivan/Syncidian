#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/plugin"
DEST="$ROOT/dist/syncidian"
ZIP="$ROOT/dist/syncidian.zip"

if [[ ! -f "$SRC/manifest.json" || ! -f "$SRC/main.js" || ! -f "$SRC/styles.css" ]]; then
  echo "Build the plugin first: (cd plugin && npm install && npm run build)"
  exit 1
fi

rm -rf "$DEST" "$ZIP"
mkdir -p "$DEST"
cp "$SRC/manifest.json" "$SRC/main.js" "$SRC/styles.css" "$DEST/"
if command -v zip >/dev/null 2>&1; then
  (cd "$ROOT/dist" && zip -qr syncidian.zip syncidian)
  echo "Wrote $ZIP"
else
  echo "Wrote $DEST (install zip to also build syncidian.zip)"
fi
echo "Attach plugin/main.js, plugin/manifest.json, and plugin/styles.css to a GitHub Release whose tag matches manifest.json version."
