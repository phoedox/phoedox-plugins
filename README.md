# Phoedox Plugins

Official plugins for [Phoedox](https://github.com/phoedox/phoedox). Each plugin is in `plugins/<name>/` and has its own README with what it does, its setup and its permissions:

```
plugins/ups/
  README.md             What it does, setup, settings, permissions
  phoedox-plugin.json   Manifest (Schema: phoedox-sdk/schema)
  backend/              Go → WASM (Extism)
  frontend/             Lit elements via @phoedox/sdk
  schemas/              JSON schemas of the kinds
  docs/                 Screenshots for the README
```

| Plugin | Content |
|---|---|
| [`ups`](plugins/ups/README.md) | UPS monitoring via NUT (example plugin) |
| [`homeassistant`](plugins/homeassistant/README.md) | Areas and devices from Home Assistant as rooms and devices; MCP `ha.state`, `ha.sync` |
| [`proxmox`](plugins/proxmox/README.md) | Nodes, VMs and LXC from Proxmox VE with "runs on"; MCP `pve.status`, `pve.sync` |
| [`github`](plugins/github/README.md) | Repositories of an organization or an account; MCP `github.open`, `github.sync` |
| [`gitlab`](plugins/gitlab/README.md) | Projects of a GitLab group (gitlab.com or self-managed); MCP `gitlab.open`, `gitlab.sync` |
| [`portainer`](plugins/portainer/README.md) | Docker environments, stacks and containers with "runs on" and "part of"; MCP `portainer.containers`, `portainer.sync` |
| [`unifi`](plugins/unifi/README.md) | UniFi gateways, switches and access points with their wired uplinks (official Network API); MCP `unifi.clients`, `unifi.sync` |
| [`fritzbox`](plugins/fritzbox/README.md) | FRITZ!Box via TR-064, optionally its active devices; MCP `fritz.hosts`, `fritz.sync` |
| [`opnsense`](plugins/opnsense/README.md) | OPNsense firewall and interfaces (address, subnet, VLAN); MCP `opnsense.leases`, `opnsense.gateways`, `opnsense.sync` |
| [`netbox`](plugins/netbox/README.md) | NetBox locations, racks and devices mounted at their unit; MCP `netbox.find`, `netbox.prefixes`, `netbox.sync` |
| [`mikrotik`](plugins/mikrotik/README.md) | MikroTik router via the RouterOS REST API; MCP `mikrotik.interfaces`, `mikrotik.leases`, `mikrotik.sync` |
| [`synology`](plugins/synology/README.md) | Synology NAS via the DSM Web API; MCP `synology.storage`, `synology.sync` |
| [`truenas`](plugins/truenas/README.md) | TrueNAS system; MCP `truenas.pools`, `truenas.shares`, `truenas.sync` |
| [`tailscale`](plugins/tailscale/README.md) | Tailscale devices, linked to the entries with the same host name; MCP `tailscale.devices`, `tailscale.sync` |
| [`kubernetes`](plugins/kubernetes/README.md) | Nodes, namespaces and workloads of a cluster; MCP `k8s.pods`, `k8s.events`, `k8s.sync` |
| [`hetzner`](plugins/hetzner/README.md) | Hetzner Cloud servers; MCP `hetzner.servers`, `hetzner.sync` |
| [`aws`](plugins/aws/README.md) | EC2 instances of chosen regions (SigV4, `ec2:DescribeInstances` only); MCP `aws.instances`, `aws.sync` |
| [`azure`](plugins/azure/README.md) | Azure virtual machines of a subscription (Reader role); MCP `azure.vms`, `azure.sync` |
| [`cloudflare`](plugins/cloudflare/README.md) | DNS zones linked to the entries their records point to; MCP `cloudflare.records`, `cloudflare.sync` |
| [`uptimekuma`](plugins/uptimekuma/README.md) | Monitors of an Uptime Kuma status page with their entries; MCP `uptimekuma.status`, `uptimekuma.down` |
| [`zabbix`](plugins/zabbix/README.md) | Zabbix problems and hosts with their entries; MCP `zabbix.problems`, `zabbix.hosts` |
| [`checkmk`](plugins/checkmk/README.md) | Checkmk service problems and host states with their entries; MCP `checkmk.problems`, `checkmk.hosts` |
| [`grafana`](plugins/grafana/README.md) | Grafana dashboards and firing alerts; MCP `grafana.dashboards`, `grafana.alerts` |
| [`shelly`](plugins/shelly/README.md) | Shelly plugs and relays (up to 10 devices); MCP `shelly.status`, `shelly.sync` |
| [`tasmota`](plugins/tasmota/README.md) | Tasmota devices (up to 10); MCP `tasmota.status`, `tasmota.sync` |
| [`hue`](plugins/hue/README.md) | Philips Hue bridge, rooms and lights (local API v1); MCP `hue.lights`, `hue.sync` |
| [`unifiprotect`](plugins/unifiprotect/README.md) | UniFi Protect NVR and cameras; MCP `protect.cameras`, `protect.sync` |
| [`frigate`](plugins/frigate/README.md) | Frigate NVR and cameras; MCP `frigate.events`, `frigate.stats`, `frigate.sync` |
| [`esphome`](plugins/esphome/README.md) | ESPHome devices from the ESPHome dashboard; MCP `esphome.devices`, `esphome.sync` |
| [`paperless`](plugins/paperless/README.md) | Search Paperless-ngx documents, also by an entry's serial or model; MCP `paperless.search`, `paperless.for_entry` |
| [`m365`](plugins/m365/README.md) | Intune devices and licences via Microsoft Graph; MCP `m365.devices`, `m365.licenses`, `m365.sync` |
| [`gworkspace`](plugins/gworkspace/README.md) | Google Workspace ChromeOS and mobile devices; MCP `gworkspace.devices`, `gworkspace.sync` |
| [`gdpr`](plugins/gdpr/README.md) | Record of processing activities (Art. 30 GDPR) as graph data; MCP `gdpr.register`, `gdpr.gaps` |
| [`isms`](plugins/isms/README.md) | ISO 27001 / BSI IT-Grundschutz controls without catalogue texts; MCP `isms.coverage`, `isms.soa` |
| [`labels`](plugins/labels/README.md) | Printable labels with QR code for Brother QL and Dymo (frontend only) |
| [`gantt`](plugins/gantt/README.md) | ```gantt block: schedules with sections, milestones, progress, dependencies and a board's due cards |
| [`matrix`](plugins/matrix/README.md) | ```matrix block: weighted decision matrix, other weights, "make it an open decision" |
| [`calc`](plugins/calc/README.md) | ```calc block: calculator with inputs, formulas and entry attributes |
| [`mindmap`](plugins/mindmap/README.md) | ```mindmap block: mind map from an outline, collapsible, copy for a whiteboard |
| [`poll`](plugins/poll/README.md) | ```poll block: readers vote, counts for everyone (never who); live on whiteboards |
| [`quiz`](plugins/quiz/README.md) | ```quiz block: questions with right/wrong answers and flashcards, kept per reader |

Integrations for self-hosted systems use `net:fetch:config`: they only call the address that the
project owner enters as `url` in the plugin settings. They only read; none of them changes the
connected system. Cloud services (GitHub, Tailscale, Hetzner, AWS, Azure, Cloudflare) can only reach their fixed API hosts. Credentials are stored encrypted as `token`
with the project.

Planned:

- **Doc types:** checklist/form, glossary, patch list/cable plan, network configuration, calendar, free diagram, process (EPC/eEPC, BPMN, swimlanes), architecture (C4), sequence diagram, database model, component, fuse box/electrical plan, map, notebook, runbook
- **Integrations:** Zammad / Jira, RoomPlan import (see the Roadmap board in Phoedox for more)

## Publishing

`scripts/pack.sh <plugin>` builds `dist/<id>-<version>.phxp` with `phx-plugin` from phoedox-sdk.
To submit a plugin to the store, raise the version in its `phoedox-plugin.json` and push a tag
`<plugin>/v<version>` (e.g. `ups/v1.3.0`). The workflow `publish.yml` checks the tag against the manifest,
runs the tests, packs and uploads with trusted publishing – no secret in the repository. Once per plugin, the
store's author area needs a trusted publisher: repository `phoedox/phoedox-plugins`, workflow `publish.yml`,
environment `store`. The optional secret `PHOEDOX_AUTHOR_KEY` adds the author signature. Every upload waits in
the review queue; CI never approves.

## License

MIT – see [LICENSE](LICENSE).
