# Cloudflare

`com.phoedox.cloudflare` · version 0.1.0

Imports the DNS zones of a Cloudflare account into the knowledge graph (status, plan, name servers, number of records and of proxied records) and links each zone to the servers, VMs, LXC containers and routers its A, AAAA and CNAME records point to ("related to", matched by IP address). AI clients can list the DNS records of a zone (type, name, content, proxied, TTL; TTL 1 means automatic). Read-only.

## What it adds

- **Kinds of entries:** `dns_zone`.
- **Tools for AI clients (MCP):**
  - `cloudflare.sync`: Import the Cloudflare DNS zones into the knowledge graph now.
  - `cloudflare.records`: DNS records of one Cloudflare zone: type, name, content, proxied and TTL (1 = automatic).
- **Runs on its own:** every 6 hours.

## Setup

1. A system administrator installs the plugin under **Administration → Plugins** (from the store, or the `.phxp` package).
2. A project owner enables it under **Project settings → Plugins** and enters the settings `token`.

**Settings:** token (an API token with the permissions Zone:Read and DNS:Read, for all zones or the zones to import).

## Permissions

- `graph:read`: reads the entries of the project.
- `graph:write:dns_zone`: creates and updates entries of the kinds `dns_zone`.
- `secrets:token`: stores the setting `token`, encrypted with the project.
- `net:fetch:api.cloudflare.com`: calls only `api.cloudflare.com`.

## Development

```bash
go test ./plugins/cloudflare/...
```

```bash
scripts/pack.sh cloudflare
```

The package lands in `dist/com.phoedox.cloudflare-0.1.0.phxp`. See the repository README for the layout of a plugin.
