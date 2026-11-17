# Zabbix

`com.phoedox.zabbix` · version 0.1.0

Gives AI clients the current problems and the hosts of a Zabbix server (6.4 or later) together with the Phoedox entry each host belongs to (matched by host name, visible name, IP address or DNS name). Read-only, no import.

## What it adds

- **Tools for AI clients (MCP):**
  - `zabbix.problems`: The 100 most recent problems (including recently resolved ones): name, severity, acknowledged, since when, hosts and the matching Phoedox entry.
  - `zabbix.hosts`: Hosts with technical and visible name, status (enabled or disabled), availability, IP address, DNS name and the matching Phoedox entry.

## Setup

1. A system administrator installs the plugin under **Administration → Plugins** (from the store, or the `.phxp` package).
2. A project owner enables it under **Project settings → Plugins** and enters the settings `url`, `token`.

**Settings:** url (the Zabbix frontend, e.g. https://zabbix.example.org; the plugin calls /api_jsonrpc.php below it) and token (an API token of a user whose role may read the hosts and problems in question; no write access needed). The plugin reads the knowledge graph only to find matching entries.

## Permissions

- `graph:read`: reads the entries of the project.
- `secrets:url`: stores the setting `url`, encrypted with the project.
- `secrets:token`: stores the setting `token`, encrypted with the project.
- `net:fetch:config`: calls only the address entered as `url` in the plugin settings.

## Development

```bash
go test ./plugins/zabbix/...
```

```bash
scripts/pack.sh zabbix
```

The package lands in `dist/com.phoedox.zabbix-0.1.0.phxp`. See the repository README for the layout of a plugin.
