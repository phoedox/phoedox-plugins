# Phoedox Plugins

Offizielle Plugins. Gesamtentscheidungen: `phoedox/phoedox` → `docs/decisions.md`; SDK und Manifest-Schema: `phoedox/phoedox-sdk`.

## Regeln

- Immer die aktuelle stabile Go-Version und aktuelle Abhängigkeiten verwenden (`go` + `toolchain` in `go.mod`).
- Ein Ordner pro Plugin unter `plugins/`, Manifest `phoedox-plugin.json` mit Reverse-DNS-ID `com.phoedox.<name>` für offizielle Plugins (Beispiel `de.example.ups` bleibt als Vorlage).
- Rechte so knapp wie möglich anfordern; jedes neue Recht im PR begründen.
- Backend: `//go:build wasip1`, Exporte per `//go:wasmexport`. Frontend: Elemente `phx-<name>`, Icons aus Lucide.
- `go test ./...` prüft alle Manifeste (Regeln aus `pdk.ParseManifest`); Backends bauen mit `GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared` (Reactor, sonst fehlen die Exporte). Logik ohne Build-Tag halten, damit sie nativ testbar ist.
- Paket bauen: `scripts/pack.sh <plugin>` → `dist/<id>-<version>.phxp` (Frontend per Bun, `@phoedox/sdk` aus `../phoedox-sdk`).
- `phoedox-sdk` ist noch privat: lokal `GOPRIVATE=github.com/phoedox`, in CI das Secret `PHOEDOX_SDK_TOKEN`.
- Plugins, die Anmeldung oder Rechte bereitstellen, setzen `"criticalPath": true`.
- Commits nach Conventional Commits mit Scope = Plugin-Name (z. B. `feat(ups): …`).
