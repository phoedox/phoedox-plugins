import { css, definePlugin, html, PhxElement } from "@phoedox/sdk";

/**
 * The mind map block, interactive (ADR-0087): an indented outline drawn with the
 * first line in the middle and its branches to both sides. Readers collapse and
 * expand branches (mouse or Enter/Space on the focused box) and copy the map onto
 * a whiteboard as shapes. Parse, layout and SVG follow backend/mindmap.go, which
 * draws the same picture for pages, the app and published pages.
 */

export interface MindNode { text: string; children: MindNode[]; id: string }
export interface Problem { key: "err_empty" | "err_too_many"; params?: Record<string, number> }

export const MAX_NODES = 400;

const esc = (s: unknown) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]!);

export function parseMindmap(src: string): { model?: MindNode; problems: Problem[] } {
  const lines = src.replace(/\r\n/g, "\n").split("\n").filter((l) => l.trim() && !l.trim().startsWith("#"));
  if (!lines.length) return { problems: [{ key: "err_empty" }] };
  const indent = (l: string) => l.replace(/\t/g, "  ").match(/^ */)![0].length;
  const clean = (l: string) => l.trim().replace(/^[-*+]\s+/, "");
  const root: MindNode = { text: clean(lines[0]), children: [], id: "0" };
  const stack: { node: MindNode; ind: number }[] = [{ node: root, ind: -1 }];
  for (const l of lines.slice(1)) {
    const ind = indent(l);
    while (stack.length > 1 && stack[stack.length - 1].ind >= ind) stack.pop();
    const parent = stack[stack.length - 1].node;
    const node: MindNode = { text: clean(l), children: [], id: `${parent.id}.${parent.children.length}` };
    parent.children.push(node);
    stack.push({ node, ind });
  }
  if (count(root) > MAX_NODES) return { problems: [{ key: "err_too_many", params: { max: MAX_NODES } }] };
  return { model: root, problems: [] };
}

function count(n: MindNode): number {
  return 1 + n.children.reduce((s, c) => s + count(c), 0);
}

const BRANCH = ["blue", "green", "orange", "purple", "pink", "red", "yellow"];
const FONT = 13, ROW = 30, GAP_X = 36;

/** A rough text width (no DOM): wide letters count more. */
export function textWidth(s: string, size = FONT): number {
  let w = 0;
  for (const ch of s) w += /[MWmw@%]/.test(ch) ? 0.9 : /[A-ZÄÖÜ0-9]/.test(ch) ? 0.68 : /[iljtf.,:;'|!]/.test(ch) ? 0.3 : 0.55;
  return w * size;
}

export interface Placed { node: MindNode; x: number; y: number; w: number; h: number; side: -1 | 0 | 1; color: string; depth: number; parent?: Placed; hidden: number }

/** Lays the map out: the root in the middle, branches split by size to the left and right. */
export function layoutMindmap(root: MindNode, collapsed: Set<string> = new Set()): { nodes: Placed[]; width: number; height: number } {
  const kids = (n: MindNode) => (collapsed.has(n.id) ? [] : n.children);
  const leaves = (n: MindNode): number => (kids(n).length ? kids(n).reduce((s, c) => s + leaves(c), 0) : 1);
  const branches = root.children;
  // Balance: fill the right side up to half of all leaves, the rest goes left.
  const total = branches.reduce((s, b) => s + leaves(b), 0);
  const right: MindNode[] = [], left: MindNode[] = [];
  let acc = 0;
  for (const b of branches) {
    if (acc < total / 2 || !right.length) { right.push(b); acc += leaves(b); } else left.push(b);
  }
  const boxW = (n: MindNode, d: number) => Math.min(240, textWidth(n.text, d === 0 ? 16 : FONT) + (d === 0 ? 32 : 20));
  // Column widths per depth and side.
  const colW: Record<string, number> = {};
  const measure = (n: MindNode, d: number, side: number) => {
    const k = `${side}:${d}`;
    colW[k] = Math.max(colW[k] ?? 0, boxW(n, d));
    for (const c of kids(n)) measure(c, d + 1, side);
  };
  right.forEach((b) => measure(b, 1, 1));
  left.forEach((b) => measure(b, 1, -1));
  const rootW = boxW(root, 0), rootH = 40;
  const colX = (side: number, d: number) => {
    let x = rootW / 2 + GAP_X;
    for (let i = 1; i < d; i++) x += (colW[`${side}:${i}`] ?? 0) + GAP_X;
    return x;
  };
  const nodes: Placed[] = [];
  const place = (n: MindNode, d: number, side: -1 | 1, top: number, color: string, parent: Placed): number => {
    const h = leaves(n) * ROW;
    const w = boxW(n, d);
    const off = colX(side, d);
    const p: Placed = { node: n, x: side === 1 ? off : -off - w, y: top + h / 2 - 12, w, h: 24, side, color, depth: d, parent, hidden: collapsed.has(n.id) ? count(n) - 1 : 0 };
    nodes.push(p);
    let y = top;
    for (const c of kids(n)) y += place(c, d + 1, side, y, color, p);
    return h;
  };
  const rp: Placed = { node: root, x: -rootW / 2, y: -rootH / 2, w: rootW, h: rootH, side: 0, color: "gray", depth: 0, hidden: 0 };
  nodes.push(rp);
  for (const [list, side] of [[right, 1], [left, -1]] as const) {
    const h = list.reduce((s, b) => s + leaves(b) * ROW, 0);
    let y = -h / 2;
    list.forEach((b) => {
      const color = BRANCH[branches.indexOf(b) % BRANCH.length];
      y += place(b, 1, side, y, color, rp);
    });
  }
  // Shift into positive coordinates with a margin.
  const minX = Math.min(...nodes.map((n) => n.x)) - 8, minY = Math.min(...nodes.map((n) => n.y)) - 8;
  const maxX = Math.max(...nodes.map((n) => n.x + n.w)) + 8, maxY = Math.max(...nodes.map((n) => n.y + n.h)) + 8;
  for (const n of nodes) { n.x -= minX; n.y -= minY; }
  return { nodes, width: maxX - minX, height: maxY - minY };
}

/** Coordinates with at most two decimals (as the backend writes them). */
const num = (f: number) => String(Math.round(f * 100) / 100);

/** Long texts end after 58 characters with an ellipsis. */
const short = (s: string) => ([...s].length > 60 ? [...s].slice(0, 58).join("") + "…" : s);

/**
 * The map as SVG; interactive adds focusable toggles for branches with children.
 * `hiddenLabel` names a collapsed branch for screen readers ("3 collapsed").
 */
export function mindmapSVG(root: MindNode, opts: { width?: number; collapsed?: Set<string>; interactive?: boolean; hiddenLabel?: (n: number) => string } = {}): string {
  const { nodes, width, height } = layoutMindmap(root, opts.collapsed);
  // 20px each side for the "+n" badges of collapsed branches.
  const vw = Math.ceil(width) + 40, vh = Math.ceil(height);
  const scale = opts.width && opts.width < vw ? opts.width / vw : 1;
  const links = nodes.filter((n) => n.parent).map((n) => {
    const p = n.parent!;
    const x1 = n.side === 1 ? p.x + p.w : p.x, y1 = p.y + p.h / 2;
    const x2 = n.side === 1 ? n.x : n.x + n.w, y2 = n.y + n.h / 2;
    const mx = (x1 + x2) / 2;
    return `<path d="M${num(x1)} ${num(y1)} C${num(mx)} ${num(y1)} ${num(mx)} ${num(y2)} ${num(x2)} ${num(y2)}" fill="none" style="stroke:var(--tc-${n.color})" stroke-width="${n.depth === 1 ? 2.5 : 1.5}"/>`;
  }).join("");
  const boxes = nodes.map((n) => {
    const toggle = opts.interactive && n.depth > 0 && n.node.children.length;
    const root = n.depth === 0;
    const label = esc(n.node.text);
    const hidden = n.hidden && opts.hiddenLabel ? ` (${esc(opts.hiddenLabel(n.hidden))})` : "";
    const attrs = toggle
      ? ` role="button" tabindex="0" data-id="${esc(n.node.id)}" aria-expanded="${!n.hidden}" aria-label="${label}${hidden}" style="cursor:pointer"`
      : "";
    const fill = root ? "var(--surface2)" : n.depth === 1 ? `var(--tb-${n.color})` : "var(--surface)";
    const stroke = root ? "var(--line)" : `var(--tc-${n.color})`;
    const bx = n.side === -1 ? n.x - 10 : n.x + n.w + 10;
    return `<g${attrs}><rect x="${num(n.x)}" y="${num(n.y)}" width="${num(n.w)}" height="${num(n.h)}" rx="${root ? 12 : 8}" style="fill:${fill};stroke:${stroke}" stroke-width="${root ? 1.5 : 1}"/>` +
      `<text x="${num(n.x + n.w / 2)}" y="${num(n.y + n.h / 2 + (root ? 5.5 : 4.5))}" text-anchor="middle" style="font-size:${root ? 16 : FONT}px;${root || n.depth === 1 ? "font-weight:600;" : ""}fill:var(--fg)">${esc(short(n.node.text))}</text>` +
      (n.hidden ? `<g><circle cx="${num(bx)}" cy="${num(n.y + n.h / 2)}" r="9" style="fill:var(--tc-${n.color})"/><text x="${num(bx)}" y="${num(n.y + n.h / 2 + 3.5)}" text-anchor="middle" style="font-size:10px;font-weight:600;fill:var(--bg)">${n.hidden > 99 ? "99+" : n.hidden}</text></g>` : "") +
      `</g>`;
  }).join("");
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="-20 0 ${vw} ${vh}" width="${Math.ceil(vw * scale)}" height="${Math.ceil(vh * scale)}" role="img" aria-label="${esc(root.text)}" style="font-family:var(--font-body,system-ui,sans-serif);max-width:100%;height:auto">${links}${boxes}</svg>`;
}

/** The map as whiteboard elements: rounded boxes and bound lines (copy and paste onto a board). */
export function mindmapElements(root: MindNode, collapsed: Set<string> = new Set()) {
  const { nodes } = layoutMindmap(root, collapsed);
  const uid = Math.random().toString(36).slice(2, 8);
  const id = (n: Placed) => `mm-${uid}-${n.node.id.replace(/\./g, "-")}`;
  const els: Record<string, unknown>[] = [];
  for (const n of nodes) {
    els.push({ id: id(n), type: n.depth === 0 ? "ellipse" : "rect", x: Math.round(n.x * 1.4), y: Math.round(n.y * 1.6), w: Math.round(n.w * 1.25), h: n.depth === 0 ? 56 : 36, text: n.node.text, stroke: n.depth === 0 ? "gray" : n.color, fill: n.depth === 1 ? n.color : undefined, round: true, fontSize: n.depth === 0 ? 20 : 16 });
  }
  for (const n of nodes) {
    if (!n.parent) continue;
    const a = els.find((e) => e.id === id(n.parent!))!, b = els.find((e) => e.id === id(n))!;
    const cx = (e: Record<string, unknown>) => (e.x as number) + (e.w as number) / 2, cy = (e: Record<string, unknown>) => (e.y as number) + (e.h as number) / 2;
    els.push({ id: `${id(n)}-l`, type: "line", points: [[cx(a), cy(a)], [cx(b), cy(b)]], stroke: n.color, start: { element: a.id }, end: { element: b.id }, smooth: true });
  }
  return els;
}

const TEXTS = {
  de: {
    label: "Mindmap",
    hidden: "{n} eingeklappt",
    expand_all: "Alles aufklappen",
    copy_board: "Für Whiteboard kopieren",
    copied: "Kopiert – auf einem Whiteboard mit Strg+V einfügen.",
    copy_failed: "Kopieren war nicht möglich.",
    err_empty: "Die Mindmap ist leer.",
    err_too_many: "Höchstens {max} Punkte.",
  },
  en: {
    label: "Mind map",
    hidden: "{n} collapsed",
    expand_all: "Expand all",
    copy_board: "Copy for a whiteboard",
    copied: "Copied – paste it on a whiteboard with Ctrl+V.",
    copy_failed: "Copying was not possible.",
    err_empty: "The mind map is empty.",
    err_too_many: "At most {max} items.",
  },
};
type TextKey = keyof typeof TEXTS.en;

class MindmapView extends PhxElement {
  static properties = {
    source: { attribute: false },
    svg: { attribute: false },
    project: { attribute: false },
    collapsed: { state: true },
    note: { state: true },
    board: { type: Boolean, reflect: true },
  };
  static styles = css`
    :host { display: block; }
    .blk { border: 1px solid var(--line); border-radius: 12px; background: var(--surface); padding: 14px 16px; color: var(--fg); font-family: var(--font-body, system-ui, sans-serif); font-size: 14px; }
    :host([board]) { height: 100%; }
    :host([board]) .blk { border: 0; border-radius: 0; height: 100%; box-sizing: border-box; overflow: auto; }
    .map { overflow-x: auto; text-align: center; }
    .map svg { max-width: none; }
    .map g[role="button"] { outline: none; }
    .map g[role="button"]:focus-visible rect { stroke: var(--accent); stroke-width: 2.5px; }
    @media (hover: hover) and (pointer: fine) { .map g[role="button"]:hover rect { filter: brightness(0.97); } }
    .foot { display: flex; gap: 8px; margin-top: 8px; flex-wrap: wrap; align-items: center; }
    .muted { color: var(--muted); font-size: 12.5px; }
    button { font: inherit; cursor: pointer; }
    button.ghost { border: 1px solid var(--line); background: var(--surface); color: var(--fg); border-radius: 8px; padding: 5px 12px; font-size: 13px; }
    button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
    @media (hover: hover) and (pointer: fine) { button.ghost:hover { background: var(--surface2); } }
    /* Nothing here animates; keep it that way for readers who ask for less motion. */
    @media (prefers-reduced-motion: reduce) { * { transition: none !important; animation: none !important; } }
  `;
  declare source: string;
  declare svg: string;
  declare project: string;
  declare collapsed: Set<string>;
  declare note: string;
  declare board: boolean;
  private shownSvg?: string;
  private parsed: ReturnType<typeof parseMindmap> = { problems: [] };
  private focusId?: string;

  constructor() {
    super();
    this.source = "";
    this.svg = "";
    this.project = "";
    this.collapsed = new Set();
    this.note = "";
    this.board = false;
  }

  connectedCallback() {
    super.connectedCallback();
    // On a whiteboard the host block carries data-board: drop the card frame there.
    const host = (this.getRootNode() as ShadowRoot).host;
    this.board = !!host?.hasAttribute("data-board");
  }

  private t(key: TextKey, params: Record<string, string | number> = {}): string {
    let lang: string;
    try {
      lang = this.sdk?.ui.locale() ?? "en";
    } catch {
      lang = "en";
    }
    const table = lang.toLowerCase().startsWith("de") ? TEXTS.de : TEXTS.en;
    return table[key].replace(/\{(\w+)\}/g, (_, k: string) => String(params[k] ?? `{${k}}`));
  }

  private get model(): MindNode | undefined {
    return this.parsed.model;
  }

  willUpdate(changed: Map<PropertyKey, unknown>) {
    // A new source starts with every branch open.
    if (changed.has("source")) {
      this.parsed = parseMindmap(this.source);
      this.collapsed = new Set();
      this.note = "";
    }
  }

  private toggle(id: string) {
    const next = new Set(this.collapsed);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    this.focusId = id;
    this.collapsed = next;
  }

  private onClick(e: Event) {
    const g = (e.target as Element).closest?.("g[data-id]") as SVGGElement | null;
    if (g?.dataset.id) this.toggle(g.dataset.id);
  }

  private onKey(e: KeyboardEvent) {
    const g = (e.target as Element).closest?.("g[data-id]") as SVGGElement | null;
    if (g?.dataset.id && (e.key === "Enter" || e.key === " ")) {
      e.preventDefault();
      this.toggle(g.dataset.id);
    }
  }

  private expandAll() {
    this.collapsed = new Set();
  }

  private async copy() {
    const m = this.model;
    if (!m) return;
    try {
      await this.sdk.ui.copyToWhiteboard(mindmapElements(m, this.collapsed));
      this.note = this.t("copied");
    } catch {
      this.note = this.t("copy_failed");
    }
  }

  updated() {
    // Our own SVG (escaped texts), or else the host's cleaned picture.
    const box = this.renderRoot.querySelector(".map");
    if (!box) return;
    const m = this.model;
    const svg = m ? mindmapSVG(m, { collapsed: this.collapsed, interactive: true, hiddenLabel: (n) => this.t("hidden", { n }) }) : this.svg;
    if (this.shownSvg !== svg) {
      box.innerHTML = svg;
      this.shownSvg = svg;
    }
    if (this.focusId) {
      (box.querySelector(`g[data-id="${CSS.escape(this.focusId)}"]`) as SVGGElement | null)?.focus();
      this.focusId = undefined;
    }
  }

  render() {
    const m = this.model;
    const problem = this.parsed.problems[0];
    return html`<div class="blk">
      <div class="map" role=${m ? "group" : "img"} aria-label=${this.t("label")} @click=${this.onClick} @keydown=${this.onKey}></div>
      ${m
        ? html`<div class="foot">
            ${this.collapsed.size ? html`<button type="button" class="ghost" @click=${this.expandAll}>${this.t("expand_all")}</button>` : ""}
            <button type="button" class="ghost" @click=${this.copy}>${this.t("copy_board")}</button>
            <span class="muted" aria-live="polite">${this.note}</span>
          </div>`
        : problem && !this.svg
          ? html`<p class="muted" role="alert">${this.t(problem.key, problem.params)}</p>`
          : ""}
    </div>`;
  }
}

export default definePlugin({
  elements: { "phx-mindmap": MindmapView },
  blocks: [{ id: "mindmap", element: "phx-mindmap", icon: "network" }],
});
