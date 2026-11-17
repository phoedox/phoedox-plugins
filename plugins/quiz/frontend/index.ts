import { css, definePlugin, html, PhxElement } from "@phoedox/sdk";
import { answersOf, lang, parseQuiz, quizKey, quizScore, type Answers, type Quiz, type QuizProblem, type QuizState } from "./quiz";

/** The plugin's own texts (from the core's quiz.* keys). */
const TEXT = {
  de: {
    label: "Quiz",
    score: "{right} von {all} richtig",
    again: "Nochmal",
    show: "Antwort zeigen",
    knew: "Gewusst",
    missed: "Nicht gewusst",
    right: "richtig",
    wrong: "falsch",
  },
  en: {
    label: "Quiz",
    score: "{right} of {all} right",
    again: "Again",
    show: "Show the answer",
    knew: "Knew it",
    missed: "Didn't know",
    right: "right",
    wrong: "wrong",
  },
};

const fill = (s: string, params: Record<string, string | number>) => s.replace(/\{(\w+)\}/g, (_, k: string) => String(params[k] ?? ""));

/**
 * The quiz, interactive (ADR-0087): readers choose answers and see right and
 * wrong with the explanation, or turn flashcards over and say whether they
 * knew them. Each reader's answers are their own: kept with
 * sdk.blocks.state ({ answers: { "<question index>": option index | "known" |
 * "unknown" } }, the core quiz's shape), under the quiz's own id or the
 * host's stateKey, so answers given to the built-in block stay.
 */
class QuizView extends PhxElement {
  static properties = {
    source: { attribute: false },
    svg: { attribute: false },
    project: { attribute: false },
    doc: { attribute: false },
    stateKey: { attribute: false },
    answers: { state: true },
    revealed: { state: true },
  };
  static styles = css`
    :host { display: block; }
    .blk { border: 1px solid var(--line); border-radius: 12px; background: var(--surface); padding: 14px 16px; color: var(--fg); font-family: var(--font-body, system-ui, sans-serif); font-size: 14px; }
    h3 { margin: 0; font: 600 16px/1.3 var(--font-display, var(--font-body, system-ui, sans-serif)); }
    .muted { color: var(--muted); font-size: 12.5px; }
    button { font: inherit; cursor: pointer; }
    button.ghost { border: 1px solid var(--line); background: var(--surface); color: var(--fg); border-radius: 8px; padding: 5px 12px; font-size: 13px; }
    button:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
    @media (hover: hover) and (pointer: fine) {
      button.ghost:hover, .opt:hover:not(:disabled) { background: var(--surface2); }
    }
    .head { display: flex; align-items: baseline; gap: 12px; margin-bottom: 8px; flex-wrap: wrap; }
    .score { margin-left: auto; font-size: 13px; color: var(--muted); }
    ol { margin: 0; padding: 0; list-style: none; counter-reset: q; display: grid; gap: 12px; }
    li.q { counter-increment: q; border-top: 1px solid var(--line); padding-top: 10px; }
    li.q:first-child { border-top: 0; padding-top: 0; }
    li.q:focus { outline: none; }
    li.q:focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; border-radius: 4px; }
    .qt { font-weight: 600; margin-bottom: 6px; }
    .qt::before { content: counter(q) ". "; color: var(--muted); }
    .opts { display: grid; gap: 5px; }
    .opt { text-align: left; border: 1px solid var(--line); border-radius: 8px; padding: 7px 12px; background: var(--surface); color: var(--fg); }
    .opt.right { border-color: var(--good); background: color-mix(in srgb, var(--good) 12%, var(--surface)); }
    .opt.wrong { border-color: var(--bad); background: color-mix(in srgb, var(--bad) 10%, var(--surface)); }
    .opt:disabled { cursor: default; }
    .card { perspective: 600px; }
    .ans { padding: 8px 12px; border-radius: 8px; background: var(--surface2); margin: 4px 0; transform-origin: top center; }
    .ex { margin-top: 6px; font-size: 13px; color: var(--muted); }
    .row { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 6px; }
    .sr { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
    .problems { margin: 0; padding-left: 18px; }
    .pic { overflow-x: auto; }
    .pic svg { display: block; max-width: 100%; height: auto; }
    @keyframes turn { from { transform: rotateX(-90deg); opacity: 0; } to { transform: none; opacity: 1; } }
    @media (prefers-reduced-motion: no-preference) {
      .ans.turn { animation: turn var(--dur, 220ms) var(--ease-out, ease-out); }
      button.ghost, .opt { transition: background-color 0.15s ease, border-color 0.15s ease; }
    }
  `;
  declare source: string;
  declare svg: string;
  declare project: string;
  declare doc: string;
  declare stateKey: string;
  /** The reader's answers (question index → option index, "known" or "unknown"). */
  declare answers: Answers;
  /** Flashcards turned over in this view, not yet marked known or not (not saved, like the core). */
  declare revealed: Set<number>;
  private model?: Quiz;
  private problems: QuizProblem[] = [];
  private parsed?: string;
  private loadedFor?: string;
  /** Counts loads and the reader's changes; a load that finishes after a change is dropped. */
  private seq = 0;
  /** What gets the focus after the next render (the clicked button may disappear or be disabled). */
  private focusNext?: string;

  constructor() {
    super();
    this.source = "";
    this.svg = "";
    this.project = "";
    this.doc = "";
    this.stateKey = "";
    this.answers = {};
    this.revealed = new Set();
  }

  private uiLang(): "de" | "en" {
    try {
      return lang(this.sdk?.ui.locale() ?? "en");
    } catch {
      return "en";
    }
  }

  private get key(): string {
    return quizKey(this.model, this.source ?? "", this.stateKey ?? "");
  }

  private store() {
    return this.sdk.blocks.state<QuizState>({ doc: this.doc ?? "", key: this.key });
  }

  willUpdate(changed: Map<PropertyKey, unknown>) {
    if (changed.has("source") && this.source !== this.parsed) {
      this.parsed = this.source;
      const { model, problems } = parseQuiz(this.source ?? "");
      this.model = model;
      this.problems = problems;
      this.answers = {};
      this.revealed = new Set();
    }
    const at = this.model ? `${this.doc ?? ""}\n${this.key}` : undefined;
    if (at !== undefined && at !== this.loadedFor) {
      this.loadedFor = at;
      void this.load();
    }
  }

  /** Reads the reader's saved answers. */
  private async load() {
    const n = ++this.seq;
    let answers: Answers = {};
    try {
      answers = answersOf((await this.store().get()).mine);
    } catch {
      /* nothing saved, or no state here */
    }
    if (n === this.seq) this.answers = answers;
  }

  private save() {
    this.seq++;
    const answers = this.answers;
    try {
      void this.store().put(Object.keys(answers).length ? { answers } : null).catch(() => undefined);
    } catch {
      /* no state here: the answers stay in this view */
    }
  }

  private answer(i: number, a: Answers[string]) {
    this.answers = { ...this.answers, [i]: a };
    this.focusNext = `li[data-i="${i}"]`;
    this.save();
  }

  /** Flashcards: turn over, then the reader says whether they knew it. */
  private reveal(i: number) {
    this.revealed = new Set(this.revealed).add(i);
    this.focusNext = `[data-knew="${i}"]`;
  }

  private again() {
    this.answers = {};
    this.revealed = new Set();
    this.focusNext = `li[data-i="0"] button`;
    this.save();
  }

  updated() {
    // Without a model (the source has problems) the server's picture is shown, if any.
    // The host cleaned this SVG (no scripts, handlers or outside links).
    const box = this.renderRoot.querySelector(".pic");
    if (box && box.getAttribute("data-svg") !== String(this.svg.length)) {
      box.innerHTML = this.svg;
      box.setAttribute("data-svg", String(this.svg.length));
    }
    if (this.focusNext) {
      this.renderRoot.querySelector<HTMLElement>(this.focusNext)?.focus();
      this.focusNext = undefined;
    }
  }

  render() {
    const t = TEXT[this.uiLang()];
    const z = this.model;
    if (!z) {
      if (this.svg) return html`<div class="pic" role="img" aria-label=${t.label}></div>`;
      return html`<div class="blk"><ul class="problems muted">${this.problems.map((p) => html`<li>${p.message}</li>`)}</ul></div>`;
    }
    const { right, done } = quizScore(z, this.answers);
    return html`<div class="blk" role="group" aria-label=${z.title || t.label}>
      <div class="head">
        ${z.title ? html`<h3>${z.title}</h3>` : ""}
        <span class="score" aria-live="polite">${fill(t.score, { right, all: z.questions.length })}</span>
        ${done ? html`<button type="button" class="ghost" @click=${() => this.again()}>${t.again}</button>` : ""}
      </div>
      <ol>${z.questions.map((q, i) => {
        const a = this.answers[i];
        const body = q.options.length
          ? html`<div class="opts" role="group" aria-label=${q.q}>${q.options.map((o, j) => {
              const cls = a === undefined ? "" : o.right ? "right" : a === j ? "wrong" : "";
              const note = a === undefined ? "" : o.right ? ` (${t.right})` : a === j ? ` (${t.wrong})` : "";
              return html`<button type="button" class="opt ${cls}" ?disabled=${a !== undefined} @click=${() => this.answer(i, j)}>${o.text}<span class="sr">${note}</span></button>`;
            })}</div>`
          : a === undefined && !this.revealed.has(i)
            ? html`<div class="row"><button type="button" class="ghost" @click=${() => this.reveal(i)}>${t.show}</button></div>`
            : html`<div class="card"><div class="ans ${this.revealed.has(i) ? "turn" : ""}">${q.answer}</div></div>${a === undefined
                ? html`<div class="row">
                    <button type="button" class="ghost" data-knew=${String(i)} @click=${() => this.answer(i, "known")}>${t.knew}</button>
                    <button type="button" class="ghost" @click=${() => this.answer(i, "unknown")}>${t.missed}</button>
                  </div>`
                : html`<div class="ex">${a === "known" ? t.knew : t.missed}</div>`}`;
        const ex = a !== undefined && q.explain ? html`<div class="ex">${q.explain}</div>` : "";
        return html`<li class="q" tabindex="-1" data-i=${String(i)}><div class="qt">${q.q}</div>${body}${ex}</li>`;
      })}</ol>
    </div>`;
  }
}

export default definePlugin({
  elements: { "phx-quiz": QuizView },
  blocks: [{ id: "quiz", element: "phx-quiz", icon: "graduation-cap" }],
});
