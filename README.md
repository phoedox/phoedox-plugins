# Phoedox Plugins

Official plugins for [Phoedox](https://github.com/phoedox/phoedox). Each plugin is in `plugins/<name>/`:

```
plugins/ups/
  phoedox-plugin.json   Manifest (Schema: phoedox-sdk/schema)
  backend/              Go → WASM (Extism)
  frontend/             Lit elements via @phoedox/sdk
  schemas/              JSON schemas of the kinds
```

| Plugin | Content |
|---|---|
| `ups` | UPS monitoring via NUT (example plugin) |
| `homeassistant` | Areas and devices from Home Assistant as rooms and devices; MCP `ha.state`, `ha.sync` |
| `proxmox` | Nodes, VMs and LXC from Proxmox VE with "runs on"; MCP `pve.status`, `pve.sync` |
| `github` | Repositories of an organization or an account; MCP `github.open`, `github.sync` |
| `gitlab` | Projects of a GitLab group (gitlab.com or self-managed); MCP `gitlab.open`, `gitlab.sync` |
| `portainer` | Docker environments, stacks and containers with "runs on" and "part of"; MCP `portainer.containers`, `portainer.sync` |
| `unifi` | UniFi gateways, switches and access points with their wired uplinks (official Network API); MCP `unifi.clients`, `unifi.sync` |
| `fritzbox` | FRITZ!Box via TR-064, optionally its active devices; MCP `fritz.hosts`, `fritz.sync` |
| `opnsense` | OPNsense firewall and interfaces (address, subnet, VLAN); MCP `opnsense.leases`, `opnsense.gateways`, `opnsense.sync` |
| `netbox` | NetBox locations, racks and devices mounted at their unit; MCP `netbox.find`, `netbox.prefixes`, `netbox.sync` |
| `mikrotik` | MikroTik router via the RouterOS REST API; MCP `mikrotik.interfaces`, `mikrotik.leases`, `mikrotik.sync` |
| `synology` | Synology NAS via the DSM Web API; MCP `synology.storage`, `synology.sync` |
| `truenas` | TrueNAS system; MCP `truenas.pools`, `truenas.shares`, `truenas.sync` |
| `tailscale` | Tailscale devices, linked to the entries with the same host name; MCP `tailscale.devices`, `tailscale.sync` |
| `kubernetes` | Nodes, namespaces and workloads of a cluster; MCP `k8s.pods`, `k8s.events`, `k8s.sync` |
| `hetzner` | Hetzner Cloud servers; MCP `hetzner.servers`, `hetzner.sync` |
| `aws` | EC2 instances of chosen regions (SigV4, `ec2:DescribeInstances` only); MCP `aws.instances`, `aws.sync` |
| `azure` | Azure virtual machines of a subscription (Reader role); MCP `azure.vms`, `azure.sync` |
| `cloudflare` | DNS zones linked to the entries their records point to; MCP `cloudflare.records`, `cloudflare.sync` |
| `uptimekuma` | Monitors of an Uptime Kuma status page with their entries; MCP `uptimekuma.status`, `uptimekuma.down` |
| `zabbix` | Zabbix problems and hosts with their entries; MCP `zabbix.problems`, `zabbix.hosts` |
| `checkmk` | Checkmk service problems and host states with their entries; MCP `checkmk.problems`, `checkmk.hosts` |
| `grafana` | Grafana dashboards and firing alerts; MCP `grafana.dashboards`, `grafana.alerts` |
| `shelly` | Shelly plugs and relays (up to 10 devices); MCP `shelly.status`, `shelly.sync` |
| `tasmota` | Tasmota devices (up to 10); MCP `tasmota.status`, `tasmota.sync` |
| `hue` | Philips Hue bridge, rooms and lights (local API v1); MCP `hue.lights`, `hue.sync` |
| `unifiprotect` | UniFi Protect NVR and cameras; MCP `protect.cameras`, `protect.sync` |
| `frigate` | Frigate NVR and cameras; MCP `frigate.events`, `frigate.stats`, `frigate.sync` |
| `esphome` | ESPHome devices from the ESPHome dashboard; MCP `esphome.devices`, `esphome.sync` |
| `paperless` | Search Paperless-ngx documents, also by an entry's serial or model; MCP `paperless.search`, `paperless.for_entry` |
| `m365` | Intune devices and licences via Microsoft Graph; MCP `m365.devices`, `m365.licenses`, `m365.sync` |
| `gworkspace` | Google Workspace ChromeOS and mobile devices; MCP `gworkspace.devices`, `gworkspace.sync` |
| `gdpr` | Record of processing activities (Art. 30 GDPR) as graph data; MCP `gdpr.register`, `gdpr.gaps` |
| `isms` | ISO 27001 / BSI IT-Grundschutz controls without catalogue texts; MCP `isms.coverage`, `isms.soa` |
| `labels` | Printable labels with QR code for Brother QL and Dymo (frontend only) |

Integrations for self-hosted systems use `net:fetch:config`: they only call the address that the
project owner enters as `url` in the plugin settings. They only read; none of them changes the
connected system. Cloud services (GitHub, Tailscale, Hetzner, AWS, Azure, Cloudflare) can only reach their fixed API hosts. Credentials are stored encrypted as `token`
with the project.

Planned:

- **Doc types:** checklist/form, glossary, patch list/cable plan, network configuration, calendar, Kanban, timeline/Gantt, free diagram, process (EPC/eEPC, BPMN, swimlanes), architecture (C4), sequence diagram, mind map/whiteboard, database model, component, fuse box/electrical plan, map, notebook, runbook
- **Integrations:** Zammad / Jira, RoomPlan import (see the Roadmap board in Phoedox for more)

## License

MIT – see [LICENSE](LICENSE).
