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

Geplant:

- **Doku-Typen:** Checkliste/Formular, Glossar, Patchliste/Kabelplan, Netzwerk-Konfiguration, Kalender, Kanban, Zeitleiste/Gantt, Freies Diagramm, Prozess (EPK/eEPK, BPMN, Swimlanes), Architektur (C4), Sequenzdiagramm, Mindmap/Whiteboard, Datenbankmodell, Komponente, Sicherungskasten/Elektroplan, Karte, Notebook, Runbook
- **Integrationen:** Proxmox & Portainer, UniFi / FRITZ!Box / OPNsense, NetBox, GitHub / GitLab, Zammad / Jira, RoomPlan-Import

## Lizenz

MIT – siehe [LICENSE](LICENSE).
