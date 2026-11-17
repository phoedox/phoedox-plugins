import { css, definePlugin, html, PhxElement, type Entity } from "@phoedox/sdk";
import { calcValues, formatValue, lang, parseCalc, type Calc, type CalcProblem, type CalcRow } from "./calc";
import { FormulaError } from "./formula";

/** The plugin's own texts (from the core's calc.* keys). */
const TEXT = {
  de: {
    label: "Rechner",
    reset: "Zurücksetzen",
    notSaved: "Deine Eingaben werden nicht gespeichert.",
    from: "Wert aus {ref}",
    unread: "Nicht lesbar: {refs}. Gib den Wert selbst ein.",
  },
  en: {
    label: "Calculator",
    reset: "Reset",
    notSaved: "Your inputs are not saved.",
    from: "Value from {ref}",
    unread: "Cannot be read: {refs}. Enter the value yourself.",
  },
};

const fill = (s: string, params: Record<string, string>) => s.replace(/\{(\w+)\}/g, (_, k: string) => params[k] ?? "");

/**
 * The calculator, interactive (ADR-0087): readers change the inputs and see
 * the results at once; nothing is saved. Inputs bound to an entry
 * ("= @nas.power_w") take the entry's attribute, read with the reader's rights.
 */
class CalcView extends PhxElement {
  static properties = {
    source: { attribute: false },
    svg: { attribute: false },
    project: { attribute: false },
    inputs: { state: true },
    loaded: { state: true },
    unread: { state: true },
  };
  static styles = css`
    :host { display: block; }
    .blk { border: 1px solid var(--line); border-radius: 12px; background: var(--surface); padding: 14px 16px; color: var(--fg); font-family: var(--font-body, system-ui, sans-serif); font-size: 14px; }
    :host([data-board]) .blk { border: 0; border-radius: 0; height: 100%; box-sizing: border-box; overflow: auto; }
    h3 { margin: 0 0 10px; font: 600 16px/1.3 var(--font-display, var(--font-body, system-ui, sans-serif)); }
    .muted { color: var(--muted); font-size: 12.5px; }
    .rows { display: grid; grid-template-columns: 1fr auto; gap: 6px 14px; align-items: center; }
    .rows label, .rows .lbl { color: var(--muted); }
    .rows .res { font-weight: 600; font-variant-numeric: tabular-nums; text-align: right; }
    .rows .lbl.res-l { color: var(--fg); font-weight: 600; }
    .rows input { width: 9em; font: inherit; text-align: right; padding: 4px 8px; border: 1px solid var(--line); border-radius: 6px; background: var(--bg); color: var(--fg); }
    .unit { color: var(--muted); font-size: 12.5px; margin-left: 4px; font-weight: 400; }
    .err { color: var(--bad); }
    .foot { margin-top: 10px; display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
    .foot:empty { display: none; }
    .note { margin: 8px 0 0; }
    .problems { margin: 0; padding-left: 18px; }
    .pic { overflow-x: auto; }
    .pic svg { display: block; max-width: none; }
    button { font: inherit; cursor: pointer; }
    button.ghost { border: 1px solid var(--line); background: var(--surface); color: var(--fg); border-radius: 8px; padding: 5px 12px; font-size: 13px; }
    button:focus-visible, input:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
    @media (hover: hover) and (pointer: fine) { button.ghost:hover { background: var(--surface2); } }
    @media (prefers-reduced-motion: no-preference) { button.ghost { transition: background-color 0.15s ease; } }
  `;
  declare source: string;
  declare svg: string;
  declare project: string;
  /** What the reader typed (name → number); cleared by reset. */
  declare inputs: Record<string, number>;
  /** Values of entry-bound inputs, from the graph. */
  declare loaded: Record<string, number>;
  /** "@entry.attr" references that could not be read. */
  declare unread: string[];
  private model?: Calc;
  private problems: CalcProblem[] = [];
  private parsed?: string;
  private seq = 0;

  constructor() {
    super();
    this.source = "";
    this.svg = "";
    this.project = "";
    this.inputs = {};
    this.loaded = {};
    this.unread = [];
  }

  private uiLang(): "de" | "en" {
    try {
      return lang(this.sdk?.ui.locale() ?? "en");
    } catch {
      return "en";
    }
  }

  willUpdate(changed: Map<PropertyKey, unknown>) {
    if (changed.has("source") && this.source !== this.parsed) {
      this.parsed = this.source;
      const { model, problems } = parseCalc(this.source ?? "");
      this.model = model;
      this.problems = problems;
      this.inputs = {};
      this.loaded = {};
      this.unread = [];
      void this.load();
    }
  }

  /** Reads the entries of bound inputs (each entry once). */
  private async load() {
    const n = ++this.seq;
    const rows = this.model?.rows.filter((r) => r.entity) ?? [];
    if (!rows.length) return;
    const entries = new Map<string, Entity | undefined>();
    await Promise.all(
      [...new Set(rows.map((r) => r.entity!.slug))].map(async (slug) => {
        try {
          entries.set(slug, await this.sdk.graph.get(slug));
        } catch {
          entries.set(slug, undefined); // missing, or the reader may not see it
        }
      }),
    );
    if (n !== this.seq) return;
    const loaded: Record<string, number> = {};
    const unread = new Set<string>();
    const inputs = { ...this.inputs };
    for (const r of rows) {
      const raw = entries.get(r.entity!.slug)?.attrs?.[r.entity!.attr];
      const v = raw == null || raw === "" ? NaN : Number(raw);
      if (Number.isNaN(v)) unread.add(r.expr);
      else {
        loaded[r.name] = v;
        delete inputs[r.name]; // the entry's value replaces what was typed before it arrived
      }
    }
    this.inputs = inputs;
    this.loaded = loaded;
    this.unread = [...unread];
  }

  /** The value an input starts with: the source's number or the entry's attribute. */
  private base(r: CalcRow): string {
    const v = r.entity ? this.loaded[r.name] : r.value;
    return v === undefined ? "" : String(v);
  }

  private onInput = (e: Event) => {
    const el = e.target as HTMLInputElement;
    const name = el.dataset.n;
    if (!name) return;
    const v = el.value === "" ? undefined : Number(el.value);
    const next = { ...this.inputs };
    if (v === undefined || Number.isNaN(v)) delete next[name];
    else next[name] = v;
    this.inputs = next;
  };

  private reset = () => {
    this.inputs = {};
    for (const el of this.renderRoot.querySelectorAll<HTMLInputElement>("input[data-n]")) {
      const r = this.model?.rows.find((x) => x.name === el.dataset.n);
      if (r) el.value = this.base(r);
    }
  };

  updated() {
    // Without a model (the source has problems) the server's picture is shown, if any.
    // The host cleaned this SVG (no scripts, handlers or outside links).
    const box = this.renderRoot.querySelector(".pic");
    if (box && box.getAttribute("data-svg") !== String(this.svg.length)) {
      box.innerHTML = this.svg;
      box.setAttribute("data-svg", String(this.svg.length));
    }
  }

  render() {
    const t = TEXT[this.uiLang()];
    const c = this.model;
    if (!c) {
      if (this.svg) return html`<div class="pic" role="img" aria-label=${t.label}></div>`;
      return html`<div class="blk"><ul class="problems muted">${this.problems.map((p) => html`<li>${p.message}</li>`)}</ul></div>`;
    }
    const vals = calcValues(c, { ...this.loaded, ...this.inputs });
    const changed = Object.keys(this.inputs).length > 0;
    const unit = (r: CalcRow) => (r.unit ? html`<span class="unit">${r.unit}</span>` : "");
    return html`<div class="blk" role="group" aria-label=${c.title || t.label}>
      ${c.title ? html`<h3>${c.title}</h3>` : ""}
      <div class="rows">
        ${c.rows.map((r, i) => {
          if (r.input) {
            return html`<label for="i${i}">${r.label}</label><span><input id="i${i}" type="number" step="any" inputmode="decimal" data-n=${r.name} .value=${this.base(r)} placeholder=${r.entity ? "–" : ""} title=${r.entity ? fill(t.from, { ref: r.expr }) : ""} @input=${this.onInput} />${unit(r)}</span>`;
          }
          const v = vals[r.name];
          return html`<span class="lbl res-l">${r.label}</span><span class="res ${v instanceof FormulaError ? "err" : ""}" aria-live="polite">${formatValue(v, this.uiLang())}${unit(r)}</span>`;
        })}
      </div>
      <div class="foot">${changed ? html`<button type="button" class="ghost" @click=${this.reset}>${t.reset}</button><span class="muted">${t.notSaved}</span>` : ""}</div>
      ${this.unread.length ? html`<p class="muted note">${fill(t.unread, { refs: this.unread.join(", ") })}</p>` : ""}
    </div>`;
  }
}

export default definePlugin({
  elements: { "phx-calc": CalcView },
  blocks: [{ id: "calc", element: "phx-calc", icon: "calculator" }],
});
