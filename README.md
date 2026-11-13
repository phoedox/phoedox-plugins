# Phoedox Plugins

Offizielle Plugins für [Phoedox](https://github.com/phoedox/phoedox). Jedes Plugin liegt in `plugins/<name>/`:

```
plugins/ups/
  phoedox-plugin.json   Manifest (Schema: phoedox-sdk/schema)
  backend/              Go → WASM (Extism)
  frontend/             Lit-Elemente über @phoedox/sdk
  schemas/              JSON-Schemas der Arten
```

| Plugin | Inhalt |
|---|---|
| `ups` | USV-Überwachung über NUT (Beispiel-Plugin) |

Geplant: Proxmox & Portainer, UniFi / FRITZ!Box / OPNsense, NetBox, GitHub / GitLab, RoomPlan-Import.
