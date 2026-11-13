# Phoedox Plugins

Offizielle Plugins. Gesamtentscheidungen: `phoedox/phoedox` → `docs/decisions.md`; SDK und Manifest-Schema: `phoedox/phoedox-sdk`.

## Regeln

- Immer die aktuelle stabile Go-Version und aktuelle Abhängigkeiten verwenden (`go` + `toolchain` in `go.mod`).
- Ein Ordner pro Plugin unter `plugins/`, Manifest `phoedox-plugin.json` mit Reverse-DNS-ID `com.phoedox.<name>` für offizielle Plugins (Beispiel `de.example.ups` bleibt als Vorlage).
- Rechte so knapp wie möglich anfordern; jedes neue Recht im PR begründen.
- Backend: `//go:build wasip1`, Exporte per `//go:wasmexport`. Frontend: Elemente `phx-<name>`, Icons aus Lucide.
- `go test ./...` prüft alle Manifeste; Backends bauen mit `GOOS=wasip1 GOARCH=wasm`.
- Plugins, die Anmeldung oder Rechte bereitstellen, setzen `"criticalPath": true`.
- Commits nach Conventional Commits mit Scope = Plugin-Name (z. B. `feat(ups): …`).
