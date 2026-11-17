// Generates the store tiles of the official plugins (proposal, see the
// "Plugin logos and screenshots" concept in Phoedox): one Lucide glyph on a
// rounded square in the colour of the plugin's category, light and dark.
// Writes docs/branding/{plugins.json,preview.html,tiles/*.svg}.
//   node scripts/tiles.mjs        (LUCIDE_ICONS overrides the icon folder)
import fs from "node:fs";
const ICONS = process.env.LUCIDE_ICONS ?? new URL("../../phoedox/web/node_modules/lucide/dist/esm/icons/", import.meta.url).pathname;
const OUT = new URL("../docs/branding/", import.meta.url).pathname;
const CATS = {
  home: { label: "Smart home", light: "#b84a0c", dark: "#f5904f" },
  network: { label: "Network", light: "#2f5bc0", dark: "#8aa8f5" },
  servers: { label: "Servers & storage", light: "#7a3eb8", dark: "#c29bf0" },
  cloud: { label: "Cloud & identity", light: "#b0306f", dark: "#ef8fbd" },
  monitor: { label: "Monitoring", light: "#2c8a4b", dark: "#6fcf8c" },
  dev: { label: "Development", light: "#6b6159", dark: "#a69b92" },
  records: { label: "Records & compliance", light: "#b4372f", dark: "#ef7a70" },
  blocks: { label: "Page blocks", light: "#8a6a00", dark: "#e9c75b" },
};
// [id, name, category, icon, previous icon if it changes]
const P = [
  ["homeassistant", "Home Assistant", "home", "house"], ["hue", "Philips Hue", "home", "lightbulb"], ["shelly", "Shelly", "home", "plug"],
  ["tasmota", "Tasmota", "home", "plug-zap"], ["esphome", "ESPHome", "home", "cpu"], ["frigate", "Frigate", "home", "cctv"], ["unifiprotect", "UniFi Protect", "home", "video", "cctv"],
  ["fritzbox", "FRITZ!Box", "network", "router"], ["mikrotik", "MikroTik", "network", "ethernet-port", "router"], ["unifi", "UniFi Network", "network", "wifi"],
  ["opnsense", "OPNsense", "network", "brick-wall-fire"], ["tailscale", "Tailscale", "network", "waypoints"], ["cloudflare", "Cloudflare", "network", "globe"], ["netbox", "NetBox", "network", "boxes", "network"],
  ["proxmox", "Proxmox VE", "servers", "server-cog"], ["truenas", "TrueNAS", "servers", "database"], ["synology", "Synology DSM", "servers", "hard-drive"],
  ["ups", "UPS (NUT)", "servers", "battery-charging"], ["hetzner", "Hetzner Cloud", "servers", "server"], ["kubernetes", "Kubernetes", "servers", "ship-wheel", "container"], ["portainer", "Portainer", "servers", "container"],
  ["aws", "AWS EC2", "cloud", "cloud"], ["azure", "Azure", "cloud", "cloud-cog", "cloud"], ["m365", "Microsoft 365", "cloud", "layout-grid", "mail"], ["gworkspace", "Google Workspace", "cloud", "at-sign", "mail"],
  ["checkmk", "Checkmk", "monitor", "circle-check-big", "activity"], ["zabbix", "Zabbix", "monitor", "activity"], ["grafana", "Grafana", "monitor", "chart-line"], ["uptimekuma", "Uptime Kuma", "monitor", "heart-pulse"],
  ["github", "GitHub", "dev", "git-pull-request"], ["gitlab", "GitLab", "dev", "git-merge"],
  ["gdpr", "GDPR records", "records", "scroll-text"], ["isms", "ISMS controls", "records", "shield-check"], ["paperless", "Paperless-ngx", "records", "file-text"], ["labels", "Labels", "records", "qr-code"],
  ["gantt", "Schedule", "blocks", "chart-gantt"], ["calc", "Calculator", "blocks", "calculator"], ["matrix", "Decision matrix", "blocks", "grid-3x3"],
  ["poll", "Poll", "blocks", "vote"], ["quiz", "Quiz", "blocks", "graduation-cap"], ["mindmap", "Mind map", "blocks", "brain-circuit", "network"],
];
fs.writeFileSync(OUT + "plugins.json", JSON.stringify(P.map(([id, name, cat, icon, old]) => ({ id, name, cat, icon, old })), null, 1));

async function glyph(name) {
  const m = await import(ICONS + name + ".mjs");
  return m.default.map(([tag, a]) => `<${tag} ${Object.entries(a).map(([k, v]) => `${k}="${v}"`).join(" ")}/>`).join("");
}

// The tile: rounded square (radius 25 %), soft fill of the category colour,
// hairline border, Lucide glyph at 55 % in the category colour.
async function tile(icon, cat, mode, size = 72) {
  const c = CATS[cat][mode], bg = mode === "light" ? "#fffdfb" : "#1f1b18";
  const g = await glyph(icon), pad = size * 0.225, s = size - 2 * pad, r = size * 0.25;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 ${size} ${size}">` +
    `<rect width="${size}" height="${size}" rx="${r}" fill="${bg}"/>` +
    `<rect width="${size}" height="${size}" rx="${r}" fill="${c}" fill-opacity="${mode === "light" ? 0.13 : 0.2}"/>` +
    `<rect x="0.5" y="0.5" width="${size - 1}" height="${size - 1}" rx="${r - 0.5}" fill="none" stroke="${c}" stroke-opacity="0.22"/>` +
    `<svg x="${pad}" y="${pad}" width="${s}" height="${s}" viewBox="0 0 24 24" fill="none" stroke="${c}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">${g}</svg></svg>`;
}

let html = `<!doctype html><meta charset="utf-8"><style>
body{margin:0;font:13px/1.35 system-ui,sans-serif}
.sheet{padding:28px 32px;display:grid;gap:14px}
.light{background:#f6f3f0;color:#2a2420}.dark{background:#171412;color:#ece6e0}
h1{font-size:20px;margin:0}
h2{font-size:12px;letter-spacing:.06em;text-transform:uppercase;margin:8px 0 0;opacity:.8;display:flex;gap:8px;align-items:center}
.dot{width:10px;height:10px;border-radius:50%}
.row{display:flex;flex-wrap:wrap;gap:14px 10px}
.p{width:112px;display:grid;justify-items:center;gap:5px;text-align:center}
.p small{font-size:11.5px;font-weight:600}.p i{font-style:normal;font-size:10.5px;opacity:.6}
.sizes{display:flex;gap:18px;align-items:end}
</style>`;
for (const mode of ["light", "dark"]) {
  html += `<div class="sheet ${mode}"><h1>Official plugin tiles · ${mode}</h1>`;
  for (const [k, c] of Object.entries(CATS)) {
    const list = P.filter((p) => p[2] === k);
    html += `<h2><span class="dot" style="background:${c[mode]}"></span>${c.label} · ${list.length}</h2><div class="row">`;
    for (const [, name, cat, icon, old] of list) html += `<div class="p">${await tile(icon, cat, mode)}<small>${name}</small><i>${icon}${old ? ` (was ${old})` : ""}</i></div>`;
    html += `</div>`;
  }
  html += `<h2>Sizes: 24 · 42 · 72 · 128</h2><div class="sizes">${await tile("house", "home", mode, 24)}${await tile("house", "home", mode, 42)}${await tile("house", "home", mode, 72)}${await tile("house", "home", mode, 128)}</div></div>`;
}
fs.writeFileSync(OUT + "preview.html", html);
for (const [id, , cat, icon] of P) fs.writeFileSync(OUT + `tiles/${id}.svg`, await tile(icon, cat, "light", 128));
