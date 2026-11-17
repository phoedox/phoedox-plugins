# Proxmox VE

`com.phoedox.proxmox` · version 0.1.0

Imports nodes, VMs and LXC containers from Proxmox VE into the knowledge graph (server, VM, LXC, "runs on") and shows AI clients their state.

## What it adds

- **Tools for AI clients (MCP):**
  - `pve.sync`: Import Proxmox nodes, VMs and containers into the knowledge graph now.
  - `pve.status`: Status, CPU, memory and uptime of all Proxmox guests and nodes.
- **Runs on its own:** every hour.

## Setup

1. A system administrator installs the plugin under **Administration → Plugins** (from the store, or the `.phxp` package).
2. A project owner enables it under **Project settings → Plugins** and enters the settings `url`, `token`.

**Settings:** url (e.g. https://pve.example.org:8006, with a valid certificate) and token (an API token in the form USER@REALM!ID=SECRET; the PVEAuditor role is enough).

## Permissions

- `graph:read`: reads the entries of the project.
- `graph:write:server,vm,lxc`: creates and updates entries of the kinds `server`, `vm`, `lxc`.
- `secrets:url`: stores the setting `url`, encrypted with the project.
- `secrets:token`: stores the setting `token`, encrypted with the project.
- `net:fetch:config`: calls only the address entered as `url` in the plugin settings.

## Development

```bash
go test ./plugins/proxmox/...
```

```bash
scripts/pack.sh proxmox
```

The package lands in `dist/com.phoedox.proxmox-0.1.0.phxp`. See the repository README for the layout of a plugin.
