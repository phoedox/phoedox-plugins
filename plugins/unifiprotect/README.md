# UniFi Protect

`com.phoedox.unifiprotect` · version 0.1.0

Imports UniFi Protect into the knowledge graph through the official integration API: the NVR (console) as a device (model, MAC, Protect version), cameras as cameras (model, MAC, connection state), floodlights as lights and sensors as devices, each "related to" the NVR. Contributes the kind camera (shared with the Frigate plugin) with the relations "located in" a room and "powered by" a switch, outlet or smart plug. AI clients get the cameras with state, model, video and HDR mode, microphone and enabled smart detections. Read-only: only GET requests. The integration API does not provide IP addresses, firmware, recording mode or last-seen times, so these camera attributes stay empty (fill them in by hand if needed).

## What it adds

- **Kinds of entries:** `camera`.
- **Relations:** `located_in`, `powered_by`.
- **Tools for AI clients (MCP):**
  - `protect.sync`: Import the UniFi Protect NVR, cameras, floodlights and sensors into the knowledge graph now.
  - `protect.cameras`: UniFi Protect cameras with connection state (connected, connecting, disconnected), model, MAC, video mode, HDR mode, microphone and enabled smart detections.
- **Runs on its own:** every 6 hours.

## Setup

1. A system administrator installs the plugin under **Administration → Plugins** (from the store, or the `.phxp` package).
2. A project owner enables it under **Project settings → Plugins** and enters the settings `url`, `apikey`.

**Settings:** url (the console, e.g. https://unifi.example.org, or the cloud connector https://api.ui.com/v1/connector/consoles/<console id> with a UniFi Site Manager API key) and apikey (UniFi OS > Control Plane > Integrations, or Site Manager > API). The certificate must be trusted by Phoedox; the console's default self-signed certificate is not, so use a console with a trusted certificate, a reverse proxy or the cloud connector. Needs UniFi Protect 5.3 or later.

## Permissions

- `graph:write:camera,device,light`: creates and updates entries of the kinds `camera`, `device`, `light`.
- `secrets:url`: stores the setting `url`, encrypted with the project.
- `secrets:apikey`: stores the setting `apikey`, encrypted with the project.
- `net:fetch:config`: calls only the address entered as `url` in the plugin settings.

## Development

```bash
go test ./plugins/unifiprotect/...
```

```bash
scripts/pack.sh unifiprotect
```

The package lands in `dist/com.phoedox.unifiprotect-0.1.0.phxp`. See the repository README for the layout of a plugin.
