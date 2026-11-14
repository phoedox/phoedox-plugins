import { css, definePlugin, html, PhxElement, type Entity } from "@phoedox/sdk";
import { encodeQr, qrPath } from "./qr";

/**
 * Printable labels for entries: name, kind, a QR code that the Phoedox app
 * recognises and chosen attributes, in common label sizes. Printing uses
 * the browser's print dialog with a CSS @page size; everything stays in the
 * browser (no network, no backend).
 */

interface Size { id: string; w: number; h: number; name: string; nameFont: number; smallFont: number }

const SIZES: Size[] = [
  { id: "ql62", w: 62, h: 29, name: "Brother QL · 62 mm continuous (62 × 29 mm)", nameFont: 9, smallFont: 6 },
  { id: "dymo54", w: 54, h: 25, name: "Dymo · 54 × 25 mm (11352)", nameFont: 8, smallFont: 5.5 },
  { id: "dymo89", w: 89, h: 36, name: "Dymo · 89 × 36 mm (99012)", nameFont: 12, smallFont: 7.5 },
];

/** Attributes that can be printed, in this order. */
const FIELDS = ["ip", "hostname", "serial", "model", "manufacturer", "mac", "asset_tag", "inventory_number"];
const MAX_FIELDS = 3;

/** Kinds whose entries are physical things that get a label. */
const KINDS = ["device", "server", "router", "repeater", "access_point", "switch", "phone", "rack", "patch_panel", "outlet", "smart_plug", "light", "room", "lan_port", "ups"];

const de = (navigator.language || "en").toLowerCase().startsWith("de");
const tr = {
  title: de ? "Etikett" : "Label",
  print: de ? "Drucken" : "Print",
  options: de ? "Optionen" : "Options",
  size: de ? "Format" : "Size",
  qr: de ? "QR-Code" : "QR code",
  qrWeb: de ? "Web-Adresse (https)" : "Web address (https)",
  qrApp: de ? "Phoedox-App (phx://)" : "Phoedox app (phx://)",
  fields: de ? "Felder (höchstens 3)" : "Fields (up to 3)",
  copies: de ? "Anzahl" : "Copies",
  noFields: de ? "Dieser Eintrag hat keine druckbaren Felder (IP, Seriennummer …)." : "This entry has no printable fields (IP, serial number …).",
  noProject: de ? "Projekt nicht erkannt – öffne den Eintrag in seinem Projekt." : "Project not found – open the entry in its project.",
  hint: de
    ? "Im Druckdialog den Etikettendrucker wählen, Ränder „Keine“ und Skalierung 100 %."
    : "In the print dialog choose the label printer, margins “None” and scale 100 %.",
  field: (k: string) => (de
    ? { ip: "IP", hostname: "Host", serial: "S/N", model: "Modell", manufacturer: "Hersteller", mac: "MAC", asset_tag: "Inventar", inventory_number: "Inv.-Nr." }
    : { ip: "IP", hostname: "Host", serial: "S/N", model: "Model", manufacturer: "Make", mac: "MAC", asset_tag: "Asset", inventory_number: "Inv. no." } as Record<string, string>)[k] ?? k,
};

/** Label styles, shared by the preview (shadow DOM) and the print copy (page). Labels are paper: black on white, not themed. */
const LABEL_CSS = `
.phxl-label { box-sizing: border-box; display: flex; align-items: center; gap: 2mm; padding: 1.5mm; overflow: hidden;
  background: #fff; color: #000; font-family: system-ui, -apple-system, "Segoe UI", Roboto, sans-serif; line-height: 1.15; }
.phxl-qr { flex: none; display: block; }
.phxl-qr path { fill: #000; }
.phxl-text { flex: 1; min-width: 0; max-height: 100%; display: flex; flex-direction: column; gap: 0.5mm; overflow: hidden; }
.phxl-name { font-weight: 700; overflow-wrap: anywhere; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.phxl-meta, .phxl-field { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.phxl-field { font-family: ui-monospace, "SF Mono", Menlo, Consolas, monospace; }
.phxl-field b { font-family: system-ui, -apple-system, "Segoe UI", Roboto, sans-serif; font-weight: 600; }
`;

export interface LabelData {
  name: string;
  meta: string;
  qr: string;
  fields: [string, string][];
}

const SVG = "http://www.w3.org/2000/svg";

/** Builds one label as DOM (text via textContent only). */
export function labelNode(d: LabelData, size: Size, doc: Document = document): HTMLElement {
  const label = doc.createElement("div");
  label.className = "phxl-label";
  label.style.width = `${size.w}mm`;
  label.style.height = `${size.h}mm`;
  const qr = encodeQr(d.qr, "M");
  const border = 1; // the label's padding adds to the quiet zone
  const n = qr.size + border * 2;
  const svg = doc.createElementNS(SVG, "svg");
  svg.setAttribute("class", "phxl-qr");
  svg.setAttribute("viewBox", `0 0 ${n} ${n}`);
  svg.setAttribute("shape-rendering", "crispEdges");
  svg.setAttribute("role", "img");
  svg.setAttribute("aria-label", d.qr);
  const side = `${size.h - 3}mm`;
  svg.setAttribute("width", side);
  svg.setAttribute("height", side);
  const path = doc.createElementNS(SVG, "path");
  path.setAttribute("d", qrPath(qr, border));
  svg.append(path);
  const text = doc.createElement("div");
  text.className = "phxl-text";
  const name = doc.createElement("div");
  name.className = "phxl-name";
  name.style.fontSize = `${size.nameFont}pt`;
  name.textContent = d.name;
  const meta = doc.createElement("div");
  meta.className = "phxl-meta";
  meta.style.fontSize = `${size.smallFont}pt`;
  meta.textContent = d.meta;
  text.append(name, meta);
  for (const [k, v] of d.fields) {
    const f = doc.createElement("div");
    f.className = "phxl-field";
    f.style.fontSize = `${size.smallFont}pt`;
    const b = doc.createElement("b");
    b.textContent = `${k} `;
    f.append(b, doc.createTextNode(v));
    text.append(f);
  }
  label.append(svg, text);
  return label;
}

const PRINT_ID = "phx-labels-print";

/** Prints labels with the page's print dialog: only the labels, one per page of the label's size. */
function printLabels(d: LabelData, size: Size, copies: number) {
  document.getElementById(PRINT_ID)?.remove();
  document.getElementById(`${PRINT_ID}-style`)?.remove();
  const style = document.createElement("style");
  style.id = `${PRINT_ID}-style`;
  style.textContent = `${LABEL_CSS}
@page { size: ${size.w}mm ${size.h}mm; margin: 0; }
@media screen { #${PRINT_ID} { display: none; } }
@media print {
  html, body { margin: 0 !important; padding: 0 !important; background: #fff !important; }
  body > *:not(#${PRINT_ID}) { display: none !important; }
  #${PRINT_ID} .phxl-label + .phxl-label { break-before: page; }
}`;
  const box = document.createElement("div");
  box.id = PRINT_ID;
  for (let i = 0; i < copies; i++) box.append(labelNode(d, size));
  document.head.append(style);
  document.body.append(box);
  const cleanup = () => {
    style.remove();
    box.remove();
    window.removeEventListener("afterprint", cleanup);
  };
  window.addEventListener("afterprint", cleanup);
  window.print();
}

/** The project of the current page (/p/<project>/…). */
function currentProject(): string {
  const m = /^\/p\/([^/]+)/.exec(location.pathname);
  return m ? decodeURIComponent(m[1]) : "";
}

/** Links a label points to; the formats the Phoedox app recognises when scanning. */
export function labelLinks(origin: string, project: string, slug: string) {
  const p = encodeURIComponent(project), s = encodeURIComponent(slug);
  return { web: `${origin}/e/${p}/${s}`, app: `phx://e/${p}/${s}` };
}

/** "access_point" → "Access point". */
export function kindLabel(kind: string): string {
  const s = kind.replace(/_/g, " ");
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** The printable attributes of an entry. */
export function printable(attrs: Record<string, unknown>): string[] {
  return FIELDS.filter((k) => ["string", "number"].includes(typeof attrs[k]) && String(attrs[k]).trim() !== "");
}

interface Prefs { size: string; target: "web" | "app"; fields: string[] }
const PREFS_KEY = "phx-labels:prefs";

function loadPrefs(): Prefs {
  const def: Prefs = { size: SIZES[0].id, target: "web", fields: ["ip", "serial"] };
  try {
    const p = JSON.parse(localStorage.getItem(PREFS_KEY) ?? "null") as Partial<Prefs> | null;
    if (!p) return def;
    return {
      size: SIZES.some((s) => s.id === p.size) ? p.size! : def.size,
      target: p.target === "app" ? "app" : "web",
      fields: Array.isArray(p.fields) ? p.fields.filter((f) => FIELDS.includes(f)) : def.fields,
    };
  } catch {
    return def;
  }
}

function savePrefs(p: Prefs) {
  try {
    localStorage.setItem(PREFS_KEY, JSON.stringify(p));
  } catch { /* private mode: keep the choice for this page only */ }
}

/** Label printing on entry pages. */
class Labels extends PhxElement {
  static properties = {
    entity: { type: String },
    e: { state: true },
    prefs: { state: true },
    copies: { state: true },
    open: { state: true },
    error: { state: true },
  };
  static styles = css`
    :host { display: block; }
    .head { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
    .head strong { display: inline-flex; gap: 8px; align-items: center; margin-right: auto; }
    select, input, button { font: inherit; font-size: 13px; color: var(--fg); background: var(--surface); border: 1px solid var(--line); border-radius: 8px; padding: 4px 8px; }
    button { cursor: pointer; }
    button.primary { background: var(--accent); border-color: var(--accent); color: var(--on-color); }
    @media (hover: hover) and (pointer: fine) { button:hover { border-color: var(--muted); } }
    .opts { display: grid; gap: 10px; margin-top: 12px; }
    .opts label { display: flex; gap: 8px; align-items: center; font-size: 13px; }
    .fields { display: flex; gap: 6px 14px; flex-wrap: wrap; }
    .row { display: flex; gap: 14px; flex-wrap: wrap; align-items: center; }
    .muted { color: var(--muted); font-size: 12px; }
    .err { color: var(--bad); font-size: 13px; }
    .preview { margin-top: 12px; overflow-x: auto; padding: 8px; background: var(--surface2); border-radius: 8px; }
    .preview .phxl-label { outline: 1px solid var(--line); }
    input[type="number"] { width: 4.5em; }
  `;
  // Declared so no class field can shadow Lit's accessors, whatever the build's class-field setting.
  declare entity: string;
  declare e?: Entity;
  declare prefs: Prefs;
  declare copies: number;
  declare open: boolean;
  declare error: string;
  private loaded = "";

  constructor() {
    super();
    this.entity = "";
    this.prefs = loadPrefs();
    this.copies = 1;
    this.open = false;
    this.error = "";
  }

  willUpdate() {
    if (this.entity && this.entity !== this.loaded) {
      this.loaded = this.entity;
      void this.load();
    }
  }

  private async load() {
    try {
      this.e = await this.sdk.graph.get(this.entity);
      this.error = "";
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err);
    }
  }

  private get size(): Size {
    return SIZES.find((s) => s.id === this.prefs.size) ?? SIZES[0];
  }

  private set(p: Partial<Prefs>) {
    this.prefs = { ...this.prefs, ...p };
    savePrefs(this.prefs);
  }

  private data(): LabelData | undefined {
    const e = this.e;
    const project = currentProject();
    if (!e || !project) return undefined;
    const links = labelLinks(location.origin, project, e.slug);
    const avail = printable(e.attrs ?? {});
    const fields = this.prefs.fields.filter((f) => avail.includes(f)).slice(0, MAX_FIELDS);
    return {
      name: e.name,
      meta: `${kindLabel(e.kind)} · @${e.slug}`,
      qr: this.prefs.target === "app" ? links.app : links.web,
      fields: fields.map((f) => [tr.field(f), String(e.attrs[f]).trim()]),
    };
  }

  private toggleField(f: string, on: boolean) {
    const list = this.prefs.fields.filter((x) => x !== f);
    this.set({ fields: on ? [...list, f] : list });
  }

  private print() {
    const d = this.data();
    if (d) printLabels(d, this.size, Math.min(Math.max(Math.round(this.copies) || 1, 1), 50));
  }

  updated() {
    const box = this.renderRoot.querySelector(".preview");
    const d = this.data();
    if (!box || !d) return;
    const style = document.createElement("style");
    style.textContent = LABEL_CSS;
    box.replaceChildren(style, labelNode(d, this.size));
  }

  render() {
    if (this.error) return html`<p class="err" role="alert">${this.error}</p>`;
    if (!this.e) return html``;
    if (!currentProject()) return html`<p class="muted">${tr.noProject}</p>`;
    const avail = printable(this.e.attrs ?? {});
    const chosen = this.prefs.fields.filter((f) => avail.includes(f));
    return html`<div class="head">
        <strong>${this.sdk.icons.render("tag", 15)} ${tr.title}</strong>
        <select aria-label=${tr.size} @change=${(ev: Event) => this.set({ size: (ev.target as HTMLSelectElement).value })}>
          ${SIZES.map((s) => html`<option value=${s.id} ?selected=${s.id === this.prefs.size}>${s.name}</option>`)}
        </select>
        <button aria-expanded=${this.open ? "true" : "false"} @click=${() => (this.open = !this.open)}>${tr.options}</button>
        <button class="primary" @click=${() => this.print()}>${tr.print}</button>
      </div>
      ${this.open ? html`<div class="opts">
        <div class="row">
          <label>${tr.qr}
            <select @change=${(ev: Event) => this.set({ target: (ev.target as HTMLSelectElement).value === "app" ? "app" : "web" })}>
              <option value="web" ?selected=${this.prefs.target === "web"}>${tr.qrWeb}</option>
              <option value="app" ?selected=${this.prefs.target === "app"}>${tr.qrApp}</option>
            </select></label>
          <label>${tr.copies} <input type="number" min="1" max="50" .value=${String(this.copies)}
            @change=${(ev: Event) => (this.copies = Number((ev.target as HTMLInputElement).value) || 1)}></label>
        </div>
        ${avail.length ? html`<div><div class="muted">${tr.fields}</div><div class="fields">
          ${avail.map((f) => html`<label><input type="checkbox" .checked=${chosen.includes(f)}
            ?disabled=${!chosen.includes(f) && chosen.length >= MAX_FIELDS}
            @change=${(ev: Event) => this.toggleField(f, (ev.target as HTMLInputElement).checked)}> ${tr.field(f)}</label>`)}
        </div></div>` : html`<p class="muted">${tr.noFields}</p>`}
        <p class="muted">${tr.hint}</p>
        <div class="preview" aria-hidden="true"></div>
      </div>` : ""}`;
  }
}

export default definePlugin({
  elements: { "phx-labels": Labels },
  views: Object.fromEntries(KINDS.map((k) => [k, "phx-labels"])),
});
