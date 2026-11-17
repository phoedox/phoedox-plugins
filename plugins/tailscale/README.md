# Tailscale

`com.phoedox.tailscale` · version 0.1.0

Imports the devices of a Tailscale tailnet into the knowledge graph as Tailscale nodes (hostname, OS, Tailscale addresses, client version, last seen, update available, tags, authorized) and links each node "related to" an existing server, VM, LXC container or device with the same hostname. AI clients can ask which devices are online, when they were last seen and whose key expires. Read-only; devices shared in from other tailnets are only listed, not imported.

## What it adds

- **Kinds of entries:** `tailscale_node`.
- **Tools for AI clients (MCP):**
  - `tailscale.sync`: Import the devices of the tailnet into the knowledge graph now.
  - `tailscale.devices`: Devices of the tailnet: online or last seen, user, OS, Tailscale IPs, client version, update available, key expiry, authorized and tags.
- **Runs on its own:** every 6 hours.

## Setup

1. A system administrator installs the plugin under **Administration → Plugins** (from the store, or the `.phxp` package).
2. A project owner enables it under **Project settings → Plugins** and enters the settings `apikey`, `tailnet`.

**Settings:** apikey (a Tailscale API access token from the admin console under Settings > Keys; it has the rights of the user who created it and expires after at most 90 days) and optional tailnet (the tailnet ID or organization name; default "-", the tailnet of the key).

## Permissions

- `graph:read`: reads the entries of the project.
- `graph:write:tailscale_node`: creates and updates entries of the kinds `tailscale_node`.
- `secrets:apikey`: stores the setting `apikey`, encrypted with the project.
- `secrets:tailnet`: stores the setting `tailnet`, encrypted with the project.
- `net:fetch:api.tailscale.com`: calls only `api.tailscale.com`.

## Development

```bash
go test ./plugins/tailscale/...
```

```bash
scripts/pack.sh tailscale
```

The package lands in `dist/com.phoedox.tailscale-0.1.0.phxp`. See the repository README for the layout of a plugin.
