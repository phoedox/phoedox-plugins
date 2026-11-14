# Phoedox Plugins

Official plugins for [Phoedox](https://github.com/phoedox/phoedox). Each plugin is in `plugins/<name>/`:

```
plugins/ups/
  phoedox-plugin.json   Manifest (Schema: phoedox-sdk/schema)
  backend/              Go → WASM (Extism)
  frontend/             Lit elements via @phoedox/sdk
  schemas/              JSON schemas of the kinds
```

| Plugin | Content |
|---|---|
| `ups` | UPS monitoring via NUT (example plugin) |
| `homeassistant` | Areas and devices from Home Assistant as rooms and devices; MCP `ha.state`, `ha.sync` |
| `proxmox` | Nodes, VMs and LXC from Proxmox VE with "runs on"; MCP `pve.status`, `pve.sync` |
| `github` | Repositories of an organization or an account; MCP `github.open`, `github.sync` |
| `gitlab` | Projects of a GitLab group (gitlab.com or self-managed); MCP `gitlab.open`, `gitlab.sync` |
| `portainer` | Docker environments, stacks and containers with "runs on" and "part of"; MCP `portainer.containers`, `portainer.sync` |
| `unifi` | UniFi gateways, switches and access points with their wired uplinks (official Network API); MCP `unifi.clients`, `unifi.sync` |
| `fritzbox` | FRITZ!Box via TR-064, optionally its active devices; MCP `fritz.hosts`, `fritz.sync` |
| `opnsense` | OPNsense firewall and interfaces (address, subnet, VLAN); MCP `opnsense.leases`, `opnsense.gateways`, `opnsense.sync` |
| `netbox` | NetBox locations, racks and devices mounted at their unit; MCP `netbox.find`, `netbox.prefixes`, `netbox.sync` |

All integrations except GitHub use `net:fetch:config`: they only call the address that the
project owner enters as `url` in the plugin settings. They only read; none of them changes the
connected system. Credentials are stored encrypted as `token`
with the project.

Planned:

- **Doc types:** checklist/form, glossary, patch list/cable plan, network configuration, calendar, Kanban, timeline/Gantt, free diagram, process (EPC/eEPC, BPMN, swimlanes), architecture (C4), sequence diagram, mind map/whiteboard, database model, component, fuse box/electrical plan, map, notebook, runbook
- **Integrations:** Zammad / Jira, RoomPlan import (see the Roadmap board in Phoedox for more)

## License

MIT – see [LICENSE](LICENSE).
