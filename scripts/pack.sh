#!/usr/bin/env bash
# Builds a plugin package: scripts/pack.sh <plugin> [outdir]
# Backend: Go -> WASM (reactor, exports via //go:wasmexport).
# Frontend: bundled with Bun (lit included); @phoedox/sdk comes from a
# phoedox-sdk checkout next to this repository (tsconfig.json paths).
set -euo pipefail
name=$1
out=${2:-dist}
src=plugins/$name
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
cp "$src/phoedox-plugin.json" "$tmp/"
[ -d "$src/schemas" ] && cp -r "$src/schemas" "$tmp/"
if [ -d "$src/backend" ]; then
  GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -trimpath -o "$tmp/backend.wasm" "./$src/backend"
fi
if [ -f "$src/frontend/index.ts" ]; then
  bun build "$src/frontend/index.ts" --format esm --minify --outfile "$tmp/frontend/index.js" >/dev/null
fi
id=$(python3 -c 'import json,sys;m=json.load(open(sys.argv[1]));print(m["id"]+"-"+m["version"])' "$src/phoedox-plugin.json")
mkdir -p "$out"
tar -C "$tmp" -czf "$out/$id.phxp" .
echo "$out/$id.phxp"
