#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Install the Syncidian Obsidian plugin into a vault (manual sideload)."
  echo
  echo "Usage: $0 /path/to/YourVault"
  echo
  echo "Then in Obsidian: Settings → Community plugins → enable Syncidian."
  exit 1
fi

VAULT="${1%/}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/plugin"
DEST="$VAULT/.obsidian/plugins/syncidian"

if [[ ! -f "$SRC/manifest.json" ]]; then
  echo "plugin/manifest.json not found"
  exit 1
fi
if [[ ! -f "$SRC/main.js" ]]; then
  echo "plugin/main.js is missing. Run: (cd plugin && npm install && npm run build)"
  exit 1
fi

mkdir -p "$DEST"
cp "$SRC/manifest.json" "$SRC/main.js" "$SRC/styles.css" "$DEST/"
echo "Installed Syncidian plugin to:"
echo "  $DEST"
echo
echo "Next:"
echo "  1. Open this vault in Obsidian"
echo "  2. Settings → Community plugins → turn community plugins on (Safe mode off)"
echo "  3. Enable Syncidian"
echo "  4. Paste your server URL and access token"
echo
echo "The plugin is not in the Community Plugin store yet. This copy step is required on every machine."
