# Phoedox Plugins

Official plugins for [Phoedox](https://github.com/phoedox/phoedox). Each plugin is in `plugins/<name>/`:

```
plugins/ups/
  phoedox-plugin.json   Manifest (Schema: phoedox-sdk/schema)
  backend/              Go → WASM (Extism)
  frontend/             Lit-Elemente über @phoedox/sdk
  schemas/              JSON-Schemas der Arten
```

| Plugin | Content |
|---|---|
| `ups` | UPS monitoring via NUT (example plugin) |
| `homeassistant` | Areas and devices from Home Assistant as rooms and devices; MCP `ha.state`, `ha.sync` |
| `proxmox` | Nodes, VMs and LXC from Proxmox VE with "runs on"; MCP `pve.status`, `pve.sync` |
| `github` | Repositories of an organization or an account; MCP `github.open`, `github.sync` |

Home Assistant and Proxmox use `net:fetch:config`: they only call the address that the
project owner enters as `url` in the plugin settings. Credentials are stored encrypted as `token`
with the project.

Planned:

- **Doc types:** checklist/form, glossary, patch list/cable plan, network configuration, calendar, Kanban, timeline/Gantt, free diagram, process (EPC/eEPC, BPMN, swimlanes), architecture (C4), sequence diagram, mind map/whiteboard, database model, component, fuse box/electrical plan, map, notebook, runbook
- **Integrations:** Proxmox & Portainer, UniFi / FRITZ!Box / OPNsense, NetBox, GitHub / GitLab, Zammad / Jira, RoomPlan import

## License

MIT – see [LICENSE](LICENSE).
