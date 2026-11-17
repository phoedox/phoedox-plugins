#!/usr/bin/env bash
# Builds a plugin package: scripts/pack.sh <plugin> [outdir]
# Uses phx-plugin from phoedox-sdk (go.mod version): checks the manifest,
# builds the backend (Go -> WASM reactor) and the frontend (Bun; @phoedox/sdk
# comes from a phoedox-sdk checkout next to this repository, tsconfig.json
# paths) and writes a reproducible <outdir>/<id>-<version>.phxp.
set -euo pipefail
name=$1
out=${2:-dist}
go run github.com/phoedox/phoedox-sdk/go/cmd/phx-plugin pack "plugins/$name" -o "$out"
