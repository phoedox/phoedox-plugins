import { css, definePlugin, html, PhxElement, type BlockState } from "@phoedox/sdk";
import { lang, myChoices, parsePoll, pollKey, share, toggleChoice, type Poll, type PollProblem } from "./poll";

/**
 * The poll block, interactive (ADR-0087): readers vote for one answer (or
 * several), change or take back their vote and see the counts of all readers,
 * never who voted. The vote is the reader's block state `{choices: [...]}`,
 * the same shape and key as the core's poll, so earlier votes keep counting.
 */

const TEXT = {
  de: {
    label: "Abstimmung",
    closed: "Abgeschlossen",
    sign_in: "Abstimmen können angemeldete Mitglieder des Projekts.",
    voters: "{n} Personen haben abgestimmt",
    voters_one: "1 Person hat abgestimmt",
    choose_one: "Wähle eine Antwort",
    choose_many: "Wähle eine oder mehrere Antworten",
    undo: "Stimme zurückziehen",
    cannot_show: "Die Abstimmung kann nicht angezeigt werden.",
    err_question: "Es fehlt die Frage.",
    err_options: "Eine Abstimmung braucht mindestens zwei Antworten („- …“).",
    err_too_many: "Höchstens 30 Antworten.",
    err_duplicate: "Die Antwort „{option}“ steht doppelt da.",
    line: "Zeile {n}",
  },
  en: {
    label: "Poll",
    closed: "Closed",
    sign_in: "Signed-in members of the project can vote.",
    voters: "{n} people voted",
    voters_one: "1 person voted",
    choose_one: "Choose one answer",
    choose_many: "Choose one or more answers",
    undo: "Take back my vote",
    cannot_show: "The poll cannot be shown.",
    err_question: "The question is missing.",
    err_options: "A poll needs at least two answers (\"- …\").",
    err_too_many: "At most 30 answers.",
    err_duplicate: "The answer \"{option}\" is there twice.",
    line: "Line {n}",
  },
};
type Key = keyof typeof TEXT.en;

/** The saved state of a reader: the chosen answers. */
interface Vote { choices?: string[] }

class PollView extends PhxElement {
  static properties = {
    source: { attribute: false },
    svg: { attribute: false },
    project: { attribute: false },
    doc: { attribute: false },
    stateKey: { attribute: false },
    mine: { state: true },
    votes: { state: true },
    voters: { state: true },
    busy: { state: true },
    loaded: { state: true },
  };
  static styles = css`
    :host { display: block; }
    :host([board]) { height: 100%; }
    .blk { border: 1px solid var(--line); border-radius: 12px; background: var(--surface); padding: 14px 16px; color: var(--fg); font-family: var(--font-body, system-ui, sans-serif); font-size: 14px; }
    :host([board]) .blk { border: 0; border-radius: 0; height: 100%; box-sizing: border-box; overflow: auto; }
    h3 { margin: 0 0 10px; font: 600 16px/1.3 var(--font-display, var(--font-body, system-ui, sans-serif)); }
    .muted { color: var(--muted); font-size: 12.5px; }
    button { font: inherit; cursor: pointer; }
    button.ghost { border: 1px solid var(--line); background: var(--surface); color: var(--fg); border-radius: 8px; padding: 5px 12px; font-size: 13px; }
    button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
    @media (hover: hover) and (pointer: fine) { button.ghost:hover:not([aria-disabled=true]) { background: var(--surface2); } }
    .opts { display: grid; gap: 6px; margin: 0; padding: 0; list-style: none; }
    .opt { position: relative; display: flex; align-items: center; gap: 10px; width: 100%; text-align: left; border: 1px solid var(--line); border-radius: 8px; padding: 9px 12px; background: var(--surface); color: var(--fg); overflow: hidden; }
    .opt[aria-checked=true] { border: 2px solid var(--accent); padding: 8px 11px; }
    .opt .bar { position: absolute; inset: 0 auto 0 0; background: var(--tb-blue); z-index: 0; transition: width var(--dur, 200ms) var(--ease-out, ease-out); }
    .opt > :not(.bar) { position: relative; z-index: 1; }
    .opt .mark { width: 16px; height: 16px; flex: none; border: 2px solid var(--muted); border-radius: 50%; box-sizing: border-box; }
    .opt.multi .mark { border-radius: 4px; }
    .opt[aria-checked=true] .mark { border-color: var(--accent); background: radial-gradient(var(--accent) 45%, transparent 50%); }
    .opt.multi[aria-checked=true] .mark { background: var(--accent); }
    .opt .n { margin-left: auto; font-variant-numeric: tabular-nums; color: var(--muted); font-size: 13px; }
    .opt:disabled, .opt[aria-disabled=true] { cursor: default; }
    .foot { display: flex; gap: 10px; align-items: center; margin-top: 10px; flex-wrap: wrap; }
    .problems { margin: 6px 0 0; padding-left: 18px; }
    .pic { overflow-x: auto; }
    .pic svg { display: block; max-width: 100%; height: auto; }
    @media (prefers-reduced-motion: reduce) { .opt .bar { transition: none; } }
  `;
  declare source: string;
  declare svg: string;
  declare project: string;
  declare doc: string;
  declare stateKey: string;
  /** The reader's choices. */
  declare mine: string[];
  /** Answer → number of readers who chose it (never who). */
  declare votes: Record<string, number>;
  declare voters: number;
  declare busy: boolean;
  /** The first state arrived (until then a click could drop earlier choices). */
  declare loaded: boolean;
  private model?: Poll;
  private problems: PollProblem[] = [];
  private parsed?: string;
  private key = "";
  private seq = 0;
  private board = false;
  private timer?: number;

  constructor() {
    super();
    this.source = "";
    this.svg = "";
    this.project = "";
    this.doc = "";
    this.stateKey = "";
    this.mine = [];
    this.votes = {};
    this.voters = 0;
    this.busy = false;
    this.loaded = false;
  }

  connectedCallback() {
    super.connectedCallback();
    // On a whiteboard the host block carries data-board: drop the card frame there.
    const host = (this.getRootNode() as ShadowRoot).host as Element | undefined;
    this.board = !!host?.hasAttribute?.("data-board");
    this.toggleAttribute("board", this.board);
    // Others vote too: look again now and then while the page is visible.
    this.timer = window.setInterval(() => {
      if (!document.hidden && !this.busy && this.remote) void this.load(false);
    }, 30000);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    window.clearInterval(this.timer);
    this.timer = undefined;
  }

  /** Votes count on the server only on a page or whiteboard (not in previews or published pages). */
  private get remote(): boolean {
    return !!(this.project && this.doc);
  }

  private uiLang(): "de" | "en" {
    try {
      return lang(this.sdk?.ui?.locale?.() ?? "en");
    } catch {
      return "en";
    }
  }

  private t(key: Key, params: Record<string, string | number> = {}): string {
    return TEXT[this.uiLang()][key].replace(/\{(\w+)\}/g, (_, p: string) => String(params[p] ?? ""));
  }

  private state() {
    return this.sdk.blocks.state<Vote>({ doc: this.doc, key: this.key });
  }

  willUpdate(changed: Map<PropertyKey, unknown>) {
    if (this.parsed !== this.source) {
      this.parsed = this.source;
      const { model, problems } = parsePoll(this.source ?? "");
      this.model = model;
      this.problems = problems;
    }
    if (changed.has("source") || changed.has("doc") || changed.has("project") || changed.has("stateKey")) {
      const key = this.model ? pollKey(this.model, this.stateKey, this.board) : "";
      if (key !== this.key) {
        this.key = key;
        this.mine = [];
        this.votes = {};
        this.voters = 0;
        this.loaded = false;
        void this.load(true);
      } else if (this.model) {
        // Same key, other answers: forget choices that are no longer answers.
        this.mine = myChoices(this.model, { choices: this.mine });
      }
    }
  }

  /** Reads the reader's vote and the counts of all readers. */
  private async load(first: boolean) {
    const key = this.key;
    if (!this.model || !key || !this.sdk?.blocks) {
      if (first) this.loaded = true;
      return;
    }
    const n = ++this.seq;
    try {
      const r = await this.state().get();
      if (n === this.seq && key === this.key) this.apply(r);
    } catch {
      /* keep the view; a first load shows the poll without votes */
    } finally {
      if (first && key === this.key) this.loaded = true;
    }
  }

  private apply(r: BlockState<Vote>) {
    if (!this.model) return;
    this.mine = myChoices(this.model, r.mine);
    this.votes = r.votes ?? {};
    this.voters = r.voters ?? 0;
  }

  /** Saves the reader's choices (null takes the vote back). */
  private async vote(next: string[] | null) {
    if (this.busy || !this.loaded) return;
    this.busy = true;
    const n = ++this.seq;
    const key = this.key;
    try {
      const r = await this.state().put(next ? { choices: next } : null);
      if (n === this.seq && key === this.key) this.apply(r);
    } catch {
      /* keep the old view */
    } finally {
      this.busy = false;
    }
  }

  private choose(option: string) {
    if (!this.model || !this.canVote) return;
    void this.vote(toggleChoice(this.model, this.mine, option));
  }

  private get canVote(): boolean {
    return !!this.model && !this.model.closed && this.remote && !!this.key;
  }

  /** Arrow keys, Home and End move between the answers (Enter and Space vote). */
  private onKey = (e: KeyboardEvent) => {
    const opts = [...this.renderRoot.querySelectorAll<HTMLButtonElement>("button.opt:not(:disabled)")];
    const i = opts.indexOf(e.target as HTMLButtonElement);
    if (i < 0) return;
    let j = -1;
    if (e.key === "ArrowDown" || e.key === "ArrowRight") j = (i + 1) % opts.length;
    else if (e.key === "ArrowUp" || e.key === "ArrowLeft") j = (i - 1 + opts.length) % opts.length;
    else if (e.key === "Home") j = 0;
    else if (e.key === "End") j = opts.length - 1;
    if (j < 0) return;
    e.preventDefault();
    opts[j].focus();
  };

  updated() {
    // Without a model the backend's picture stands in, if any (the host cleaned this SVG).
    const box = this.renderRoot.querySelector(".pic");
    if (box && box.getAttribute("data-svg") !== String(this.svg.length)) {
      box.innerHTML = this.svg;
      box.setAttribute("data-svg", String(this.svg.length));
    }
  }

  private problemText(p: PollProblem): string {
    const text = this.t(p.key, p.params ?? {});
    return p.line > 1 ? `${this.t("line", { n: p.line })}: ${text}` : text;
  }

  render() {
    const p = this.model;
    if (!p) {
      if (this.svg) return html`<div class="pic" role="img" aria-label=${this.t("label")}></div>`;
      return html`<div class="blk" role="alert"><b>${this.t("cannot_show")}</b>
        <ul class="problems muted">${this.problems.map((x) => html`<li>${this.problemText(x)}</li>`)}</ul></div>`;
    }
    const show = p.closed || p.results === "always" || this.mine.length > 0;
    const canVote = this.canVote;
    const waiting = this.busy || !this.loaded;
    const status = p.closed
      ? this.t("closed")
      : !this.remote
        ? this.t("sign_in")
        : show
          ? this.t(this.voters === 1 ? "voters_one" : "voters", { n: this.voters })
          : this.t(p.multiple ? "choose_many" : "choose_one");
    return html`<div class="blk" aria-busy=${waiting ? "true" : "false"}>
      <h3 id="q">${p.question}</h3>
      <div class="opts" role=${p.multiple ? "group" : "radiogroup"} aria-labelledby="q" @keydown=${this.onKey}>
        ${p.options.map((o) => {
          const { n, pct } = share(p, this.votes, this.voters, o);
          const on = this.mine.includes(o);
          return html`<button type="button" class="opt${p.multiple ? " multi" : ""}" role=${p.multiple ? "checkbox" : "radio"}
              aria-checked=${on ? "true" : "false"} ?disabled=${!canVote} aria-disabled=${canVote && waiting ? "true" : "false"}
              @click=${() => { if (!waiting) this.choose(o); }}>
            ${show ? html`<span class="bar" style="width:${pct}%"></span>` : ""}<span class="mark" aria-hidden="true"></span><span>${o}</span>${show ? html`<span class="n">${n} · ${pct}%</span>` : ""}
          </button>`;
        })}
      </div>
      <div class="foot">
        <span class="muted" aria-live="polite">${status}</span>
        ${this.mine.length && canVote
          ? html`<button type="button" class="ghost" aria-disabled=${waiting ? "true" : "false"} @click=${() => { if (!waiting) void this.vote(null); }}>${this.t("undo")}</button>`
          : ""}
      </div>
    </div>`;
  }
}

export default definePlugin({
  elements: { "phx-poll": PollView },
  blocks: [{ id: "poll", element: "phx-poll", icon: "vote" }],
});
