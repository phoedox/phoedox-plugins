# Frigate

`com.phoedox.frigate` · version 0.1.0

Imports a Frigate NVR into the knowledge graph: Frigate as a service (URL, port, version) and its cameras as cameras (IP address or host name from the stream or ONVIF address, state, detect resolution, recording on/off, object detection, tracked objects), each "related to" the service. Contributes the kind camera (shared with the UniFi Protect plugin) with the relations "located in" a room and "powered by" a switch, outlet or smart plug. AI clients get the 50 most recent events (label, camera, score, start and end time, clip and snapshot available; optionally filtered by camera or label) and statistics (version, camera and detection rates, detector inference time, CPU and memory, storage). Read-only: only GET requests and the login. Stream credentials are never stored; only the host is read from stream addresses.

## What it adds

- **Kinds of entries:** `camera`.
- **Relations:** `located_in`, `powered_by`.
- **Tools for AI clients (MCP):**
  - `frigate.sync`: Import Frigate and its cameras into the knowledge graph now.
  - `frigate.events`: The 50 most recent Frigate events: label (and sub label), camera, score in percent, start and end time (UTC), zones, whether a clip and a snapshot exist. Optional filters camera and label.
  - `frigate.stats`: Frigate statistics: version and latest version, uptime, frames and detections per second per camera, connection quality, detector inference time, CPU and memory use and storage usage in GiB.
- **Runs on its own:** every 6 hours.

## Setup

1. A system administrator installs the plugin under **Administration → Plugins** (from the store, or the `.phxp` package).
2. A project owner enables it under **Project settings → Plugins** and enters the settings `url`, `username`, `password`, `token`, `name`.

**Settings:** url (e.g. https://frigate.example.org:8971, or http://<host>:5000 for the unauthenticated internal port), optional username and password (a Frigate user, 0.14 or later; the plugin logs in with POST /api/login on each run and uses the session token, a viewer role is enough), optional token (a fixed bearer token instead; Frigate session tokens expire, so username and password are preferred) and optional name (default: Frigate).

## Permissions

- `graph:write:service,camera`: creates and updates entries of the kinds `service`, `camera`.
- `secrets:url`: stores the setting `url`, encrypted with the project.
- `secrets:username`: stores the setting `username`, encrypted with the project.
- `secrets:password`: stores the setting `password`, encrypted with the project.
- `secrets:token`: stores the setting `token`, encrypted with the project.
- `secrets:name`: stores the setting `name`, encrypted with the project.
- `net:fetch:config`: calls only the address entered as `url` in the plugin settings.

## Development

```bash
go test ./plugins/frigate/...
```

```bash
scripts/pack.sh frigate
```

The package lands in `dist/com.phoedox.frigate-0.1.0.phxp`. See the repository README for the layout of a plugin.
