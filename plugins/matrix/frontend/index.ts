import { css, definePlugin, html, PhxElement } from "@phoedox/sdk";

/**
 * The decision matrix block, interactive (ADR-0087): options × weighted criteria.
 * Scores are added up with their weights; readers can try other weights (not
 * saved), and the options can become the options of an open decision (ADR-0062).
 * The source is read here the same way the backend reads it for the picture.
 */

export interface Criterion { name: string; weight: number }
export interface MatrixOption { name: string; scores: number[] }
export interface Matrix { title: string; criteria: Criterion[]; options: MatrixOption[]; max: number }

const KEYS = ["title", "titel", "criteria", "kriterien", "scale", "skala"];

/** Reads a matrix; without a model when the source has problems (the backend names them). */
export function parseMatrix(src: string): { model?: Matrix; problems: number } {
  const m: Matrix = { title: "", criteria: [], options: [], max: 0 };
  let problems = 0;
  let scaleMax = 0;
  const lines = src.replace(/\r\n/g, "\n").split("\n").map((l) => l.trim()).filter((l) => l && !l.startsWith("#"));
  for (const l of lines) {
    const kv = /^([A-Za-z]+)\s*:\s*(.*)$/.exec(l);
    const k = kv?.[1].toLowerCase() ?? "";
    if (kv && KEYS.includes(k)) {
      const v = kv[2].trim();
      if (k === "title" || k === "titel") m.title = v;
      else if (k === "scale" || k === "skala") scaleMax = Number(/(\d+(?:[.,]\d+)?)\s*$/.exec(v)?.[1]?.replace(",", ".") ?? 0);
      else m.criteria = v.split(",").map((c) => c.trim()).filter(Boolean).map((c) => {
        const w = /^(.*?)\s*[*×x]\s*(\d+(?:[.,]\d+)?)$/.exec(c);
        return w ? { name: w[1].trim(), weight: Number(w[2].replace(",", ".")) } : { name: c, weight: 1 };
      });
      continue;
    }
    const o = /^[-*+]\s+(.+?)\s*:\s*(.+)$/.exec(l);
    if (!o) continue;
    const parts = o[2].split(/[,;]/).map((x) => x.trim());
    const scores = parts.map((x) => Number(x.replace(",", ".")));
    if (scores.some((x, i) => parts[i] === "" || !Number.isFinite(x))) problems++;
    m.options.push({ name: o[1], scores });
  }
  if (!m.criteria.length) problems++;
  if (m.options.length < 2) problems++;
  for (const o of m.options) if (m.criteria.length && o.scores.length !== m.criteria.length) problems++;
  m.max = scaleMax || Math.max(5, ...m.options.flatMap((o) => o.scores.filter((x) => Number.isFinite(x))));
  return problems ? { problems } : { model: m, problems };
}

/** Each option's share of the best possible result (0–1) with the given weights. */
export function matrixScores(m: Matrix, weights = m.criteria.map((c) => c.weight)): number[] {
  const wsum = weights.reduce((a, b) => a + b, 0) || 1;
  return m.options.map((o) => o.scores.reduce((s, x, i) => s + x * (weights[i] ?? 0), 0) / (wsum * m.max));
}

const TEXT = {
  de: {
    label: "Entscheidungsmatrix",
    option: "Option",
    total: "Gesamt",
    tune: "Gewichte ausprobieren",
    tune_done: "Fertig",
    reset: "Zurücksetzen",
    not_saved: "Nur für dich, wird nicht gespeichert.",
    weight_of: "Gewicht von {name}",
    to_decision: "Als offene Entscheidung anlegen",
    decision_title: "Entscheidung",
    decision_context: "Angelegt aus einer Entscheidungsmatrix; die Bewertungen stehen bei den Optionen.",
    decision_made: "Offene Entscheidung angelegt",
    decision_failed: "Die Entscheidung konnte nicht angelegt werden.",
  },
  en: {
    label: "Decision matrix",
    option: "Option",
    total: "Total",
    tune: "Try other weights",
    tune_done: "Done",
    reset: "Reset",
    not_saved: "Just for you, not saved.",
    weight_of: "Weight of {name}",
    to_decision: "Make it an open decision",
    decision_title: "Decision",
    decision_context: "Made from a decision matrix; the scores are listed with the options.",
    decision_made: "Open decision created",
    decision_failed: "The decision could not be created.",
  },
};
type Key = keyof typeof TEXT.en;

class MatrixView extends PhxElement {
  static properties = {
    source: { attribute: false },
    svg: { attribute: false },
    project: { attribute: false },
    weights: { state: true },
    tuning: { state: true },
    busy: { state: true },
  };
  static styles = css`
    :host { display: block; }
    :host([board]) { height: 100%; }
    .blk { border: 1px solid var(--line); border-radius: 12px; background: var(--surface); padding: 14px 16px; color: var(--fg); font-family: var(--font-body, system-ui, sans-serif); font-size: 14px; }
    :host([board]) .blk { border: 0; border-radius: 0; height: 100%; box-sizing: border-box; overflow: auto; }
    h3 { margin: 0 0 10px; font: 600 16px/1.3 var(--font-display, var(--font-body)); }
    .muted { color: var(--muted); font-size: 12.5px; }
    button { font: inherit; cursor: pointer; }
    button.ghost { border: 1px solid var(--line); background: var(--surface); color: var(--fg); border-radius: 8px; padding: 5px 12px; font-size: 13px; }
    button:disabled { opacity: .6; cursor: default; }
    button:focus-visible, input:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
    @media (hover: hover) and (pointer: fine) { button.ghost:hover:not(:disabled) { background: var(--surface2); } }
    .wrap { overflow-x: auto; }
    table { border-collapse: collapse; width: 100%; font-size: 13.5px; }
    th, td { padding: 6px 8px; border-bottom: 1px solid var(--line); text-align: center; white-space: nowrap; }
    th:first-child, td:first-child { text-align: left; white-space: normal; }
    th { font-weight: 600; color: var(--muted); font-size: 12.5px; }
    th .w { display: block; font-weight: 400; }
    th input { width: 3.4em; font: inherit; text-align: center; border: 1px solid var(--line); border-radius: 6px; background: var(--bg); color: var(--fg); padding: 2px; }
    td.total { min-width: 150px; text-align: left; }
    .bar { display: inline-block; height: 10px; border-radius: 5px; background: var(--tc-gray); vertical-align: middle; margin-right: 6px; transition: width var(--dur, 200ms) var(--ease-out, ease-out); }
    tr.best td { background: color-mix(in srgb, var(--good) 9%, transparent); }
    tr.best .bar { background: var(--good); }
    tr.best td:first-child { font-weight: 600; }
    .foot { display: flex; gap: 8px; margin-top: 10px; flex-wrap: wrap; align-items: center; }
    .pic { overflow-x: auto; }
    .pic svg { display: block; max-width: 100%; height: auto; }
    @media (prefers-reduced-motion: reduce) { .bar { transition: none; } }
  `;
  declare source: string;
  declare svg: string;
  declare project: string;
  declare weights: number[];
  declare tuning: boolean;
  declare busy: boolean;
  private model?: Matrix;
  private parsed?: string;
  private board = false;

  constructor() {
    super();
    this.source = "";
    this.svg = "";
    this.project = "";
    this.weights = [];
    this.tuning = false;
    this.busy = false;
  }

  connectedCallback() {
    super.connectedCallback();
    // On a whiteboard the host block carries data-board; decisions are made from pages only.
    const host = (this.getRootNode() as ShadowRoot).host as Element | undefined;
    this.board = !!host?.hasAttribute?.("data-board");
    this.toggleAttribute("board", this.board);
  }

  private t(key: Key, params: Record<string, string | number> = {}): string {
    let lang = "en";
    try {
      lang = (this.sdk?.ui?.locale?.() ?? "en").slice(0, 2).toLowerCase();
    } catch {
      /* no locale: English */
    }
    const table: Record<Key, string> = lang in TEXT ? TEXT[lang as keyof typeof TEXT] : TEXT.en;
    return table[key].replace(/\{(\w+)\}/g, (_, p: string) => String(params[p] ?? ""));
  }

  willUpdate() {
    if (this.parsed === this.source) return;
    this.parsed = this.source;
    this.model = parseMatrix(this.source).model;
    this.weights = this.model?.criteria.map((c) => c.weight) ?? [];
  }

  updated() {
    // Without a model the backend's picture stands in (the host cleaned this SVG).
    const box = this.renderRoot.querySelector(".pic");
    if (box && box.getAttribute("data-svg") !== String(this.svg.length)) {
      box.innerHTML = this.svg;
      box.setAttribute("data-svg", String(this.svg.length));
    }
    // Inputs the reader typed in keep their own value; after a reset they show the weights again.
    this.renderRoot.querySelectorAll<HTMLInputElement>("input[data-w]").forEach((el) => {
      const w = this.weights[Number(el.dataset.w)];
      if ((this.renderRoot as ShadowRoot).activeElement !== el && Number(el.value) !== w) el.value = String(w);
    });
  }

  private setWeight(i: number, value: string) {
    const w = [...this.weights];
    w[i] = Math.max(0, Number(value) || 0);
    this.weights = w;
  }

  private reset() {
    this.weights = this.model?.criteria.map((c) => c.weight) ?? [];
  }

  /** An open decision whose options are the matrix's options (people choose in Phoedox). */
  private async toDecision() {
    const m = this.model;
    if (!m || this.busy) return;
    const scores = matrixScores(m, this.weights);
    this.busy = true;
    try {
      const r = await this.sdk.docs.create("adr", m.title || this.t("decision_title"), {
        adr: {
          status: "proposed",
          context: this.t("decision_context"),
          options: m.options.map((o, i) => ({
            title: o.name,
            description: `${m.criteria.map((c, j) => `- ${c.name} (× ${this.weights[j]}): ${o.scores[j]}`).join("\n")}\n\n**${this.t("total")}: ${Math.round(scores[i] * 100)} %**`,
          })),
        },
      });
      this.sdk.ui.toast(this.t("decision_made"));
      this.sdk.ui.open(`d/${r.slug}`);
    } catch {
      this.sdk.ui.toast(this.t("decision_failed"), "error");
    } finally {
      this.busy = false;
    }
  }

  render() {
    const m = this.model;
    if (!m) return html`<div class="pic" role="img" aria-label=${this.t("label")}></div>`;
    const scores = matrixScores(m, this.weights);
    const best = Math.max(...scores);
    const changed = this.weights.some((w, i) => w !== m.criteria[i].weight);
    const pct = (s: number) => Math.round(s * 100);
    return html`<div class="blk">
      ${m.title ? html`<h3>${m.title}</h3>` : ""}
      <div class="wrap"><table>
        <thead><tr>
          <th scope="col">${this.t("option")}</th>
          ${m.criteria.map((c, i) => html`<th scope="col">${c.name}<span class="w">${this.tuning
            ? html`<input type="number" min="0" max="10" step="1" value=${String(this.weights[i])} data-w=${String(i)}
                aria-label=${this.t("weight_of", { name: c.name })} @input=${(e: Event) => this.setWeight(i, (e.target as HTMLInputElement).value)}>`
            : `× ${this.weights[i]}`}</span></th>`)}
          <th scope="col">${this.t("total")}</th>
        </tr></thead>
        <tbody>${m.options.map((o, i) => html`<tr class=${scores[i] === best ? "best" : ""}>
          <td>${o.name}</td>${o.scores.map((x) => html`<td>${x}</td>`)}
          <td class="total"><span class="bar" style="width:${pct(scores[i])}px"></span>${pct(scores[i])} %</td>
        </tr>`)}</tbody>
      </table></div>
      <div class="foot">
        <button type="button" class="ghost" aria-pressed=${String(this.tuning)} @click=${() => { this.tuning = !this.tuning; }}>${this.t(this.tuning ? "tune_done" : "tune")}</button>
        ${changed ? html`<button type="button" class="ghost" @click=${() => this.reset()}>${this.t("reset")}</button><span class="muted">${this.t("not_saved")}</span>` : ""}
        ${this.project && !this.board ? html`<button type="button" class="ghost" ?disabled=${this.busy} @click=${() => void this.toDecision()}>${this.t("to_decision")}</button>` : ""}
      </div>
    </div>`;
  }
}

export default definePlugin({
  elements: { "phx-matrix": MatrixView },
  blocks: [{ id: "matrix", element: "phx-matrix", icon: "grid-3x3" }],
});
