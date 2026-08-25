#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Sideload the Syncidian Obsidian plugin into a vault."
  echo
  echo "Preferred: Settings → Community plugins → Browse → Syncidian."
  echo "If it is not listed yet, install BRAT and add shangeethsivan/Syncidian,"
  echo "or use this script:"
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
echo "If Community plugins → Browse already lists Syncidian, install from there instead of copying files."
echo
echo "Android / iOS: copy the same three files into the vault on the device, or install from Community plugins / BRAT."
echo "On a phone, Server URL must be public HTTPS — localhost is the phone, not your computer."
echo "Unlike Git community plugins, Syncidian is not desktop-only."
