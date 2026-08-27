#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Sideload the Syncidian Obsidian plugin into a vault."
  echo
  echo "Preferred: Settings → Community plugins → Browse → Syncidian."
  echo "If it is not listed yet, install BRAT and add shangeethsivan/Syncidian,"
  echo "or use this script:"
  echo
  echo "Usage: $0 /path/to/YourVault [https://your-syncidian.example]"
  echo
  echo "With only a vault path, copies plugin/ from this repo."
  echo "With a server URL, downloads main.js, manifest.json, and styles.css from"
  echo "that instance (GET /assets/obsidian/) so you do not need a git clone."
  echo
  echo "Then in Obsidian: Settings → Community plugins → enable Syncidian."
  exit 1
fi

VAULT="${1%/}"
DEST="$VAULT/.obsidian/plugins/syncidian"
mkdir -p "$DEST"

if [[ $# -ge 2 ]]; then
  BASE="${2%/}"
  echo "Downloading plugin from ${BASE}/assets/obsidian/"
  for f in main.js manifest.json styles.css; do
    url="${BASE}/assets/obsidian/${f}"
    if command -v curl >/dev/null 2>&1; then
      curl -fsSL "$url" -o "$DEST/$f"
    elif command -v wget >/dev/null 2>&1; then
      wget -qO "$DEST/$f" "$url"
    else
      echo "Need curl or wget to download from the server"
      exit 1
    fi
  done
else
  ROOT="$(cd "$(dirname "$0")/.." && pwd)"
  SRC="$ROOT/plugin"
  if [[ ! -f "$SRC/manifest.json" ]]; then
    echo "plugin/manifest.json not found"
    exit 1
  fi
  if [[ ! -f "$SRC/main.js" ]]; then
    echo "plugin/main.js is missing. Run: (cd plugin && npm install && npm run build)"
    exit 1
  fi
  cp "$SRC/manifest.json" "$SRC/main.js" "$SRC/styles.css" "$DEST/"
fi

echo "Installed Syncidian plugin to:"
echo "  $DEST"
echo
echo "Next:"
echo "  1. Open this vault in Obsidian"
echo "  2. Settings → Community plugins → turn community plugins on (Safe mode off)"
echo "  3. Enable Syncidian"
echo "  4. Paste your server URL and access token"
echo
echo "To refresh from a running server without cloning the repo:"
echo "  $0 \"$VAULT\" https://your-syncidian.example"
echo "Or open https://your-syncidian.example/assets/obsidian.zip and unzip into that folder."
echo
echo "If Community plugins → Browse already lists Syncidian, install from there instead of copying files."
echo
echo "Android / iOS: copy the same three files into the vault on the device, or install from Community plugins / BRAT."
echo "On a phone, Server URL must be public HTTPS — localhost is the phone, not your computer."
echo "Unlike Git community plugins, Syncidian is not desktop-only."
