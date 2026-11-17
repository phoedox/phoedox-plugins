# UPS monitoring

`de.example.ups` · version 1.2.0

Reads charge, runtime and status of UPS devices through the HTTP API of a NUT server (e.g. PeaNUT) and shows them on the entry. This is the example plugin of the SDK.

## What it adds

- **Kinds of entries:** `ups`.
- **Relations:** `powers`.
- **Tools for AI clients (MCP):**
  - `ups.status`: Charge, runtime and status of one UPS or all UPS of the project.
- **Runs on its own:** every 60 seconds.
- **Frontend:** adds elements to the web app.

## Setup

1. A system administrator installs the plugin under **Administration → Plugins** (from the store, or the `.phxp` package).
2. A project owner enables it under **Project settings → Plugins** and enters the settings `nut_password`.

## Permissions

- `graph:read`: reads the entries of the project.
- `graph:write:ups`: creates and updates entries of the kinds `ups`.
- `secrets:nut_password`: stores the setting `nut_password`, encrypted with the project.
- `net:fetch:nut.lan`: calls only `nut.lan`.

## Development

```bash
go test ./plugins/ups/...
```

```bash
scripts/pack.sh ups
```

The package lands in `dist/de.example.ups-1.2.0.phxp`. See the repository README for the layout of a plugin.
