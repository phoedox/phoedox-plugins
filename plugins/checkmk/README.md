# Checkmk

`com.phoedox.checkmk` · version 0.1.0

Gives AI clients the service problems and the host states of a Checkmk site (2.1 or later, REST API) together with the Phoedox entry each host belongs to (matched by host name or address). Read-only, no import.

## What it adds

- **Tools for AI clients (MCP):**
  - `checkmk.problems`: Services that are not OK (WARN, CRIT, UNKNOWN): host, service, state, plugin output, since when, acknowledged and the matching Phoedox entry.
  - `checkmk.hosts`: Hosts with address, state (UP, DOWN, UNREACHABLE) and the matching Phoedox entry.

## Setup

1. A system administrator installs the plugin under **Administration → Plugins** (from the store, or the `.phxp` package).
2. A project owner enables it under **Project settings → Plugins** and enters the settings `url`, `username`, `secret`.

**Settings:** url (the site address, e.g. https://monitoring.example.org/mysite), username and secret (an automation user that may see the hosts and services in question; monitoring read access is enough). The plugin reads the knowledge graph only to find matching entries.

## Permissions

- `graph:read`: reads the entries of the project.
- `secrets:url`: stores the setting `url`, encrypted with the project.
- `secrets:username`: stores the setting `username`, encrypted with the project.
- `secrets:secret`: stores the setting `secret`, encrypted with the project.
- `net:fetch:config`: calls only the address entered as `url` in the plugin settings.

## Development

```bash
go test ./plugins/checkmk/...
```

```bash
scripts/pack.sh checkmk
```

The package lands in `dist/com.phoedox.checkmk-0.1.0.phxp`. See the repository README for the layout of a plugin.
