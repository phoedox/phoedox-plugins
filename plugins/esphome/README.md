# ESPHome

`com.phoedox.esphome` · version 0.1.0

Imports the devices of an ESPHome dashboard (Device Builder) into the knowledge graph as devices (board, platform, IP address, MAC, host name, deployed ESPHome version, configuration file, comment), "located in" the room named by their area (an existing room with the same name is reused, otherwise one is created). AI clients get every device with online state, versions, available update, web server and integrations, plus discovered devices that are not adopted yet. Read-only: only GET /devices and /ping; nothing is compiled, flashed or switched. The devices themselves are not contacted: the native API is protobuf over TCP and the web_server component has no device information endpoint, so the dashboard is the only clean JSON source; /devices is the endpoint the Home Assistant integration uses and may change when the dashboard drops it.

## What it adds

- **Tools for AI clients (MCP):**
  - `esphome.sync`: Import the ESPHome devices into the knowledge graph now.
  - `esphome.devices`: ESPHome devices with online state, address, IP, platform, board, area, deployed and current ESPHome version, update available, web server, MQTT, deep sleep and integrations; plus discovered devices not adopted yet.
- **Runs on its own:** every 6 hours.

## Setup

1. A system administrator installs the plugin under **Administration → Plugins** (from the store, or the `.phxp` package).
2. A project owner enables it under **Project settings → Plugins** and enters the settings `url`, `username`, `password`.

**Settings:** url (the dashboard, e.g. http://192.168.1.40:6052; the Home Assistant add-on must expose its port, ingress is not reachable) and optional username and password (the dashboard login, sent as basic authentication; the dashboard before ESPHome 2026.6 only works without a password).

## Permissions

- `graph:read`: reads the entries of the project.
- `graph:write:device,room`: creates and updates entries of the kinds `device`, `room`.
- `secrets:url`: stores the setting `url`, encrypted with the project.
- `secrets:username`: stores the setting `username`, encrypted with the project.
- `secrets:password`: stores the setting `password`, encrypted with the project.
- `net:fetch:config`: calls only the address entered as `url` in the plugin settings.

## Development

```bash
go test ./plugins/esphome/...
```

```bash
scripts/pack.sh esphome
```

The package lands in `dist/com.phoedox.esphome-0.1.0.phxp`. See the repository README for the layout of a plugin.
