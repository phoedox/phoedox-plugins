import { css, definePlugin, html, PhxElement, type Task } from "@phoedox/sdk";

/**
 * The schedule block, interactive (ADR-0086): starts with the server's picture,
 * then follows the block's width and adds the cards of `board: <slug>` that have a
 * due date (from the day they were created to the due date), drawn again by the
 * plugin's backend so pages, the app and published pages show the same thing.
 */
class GanttView extends PhxElement {
  static properties = { source: { attribute: false }, svg: { attribute: false }, project: { attribute: false }, note: { state: true } };
  static styles = css`
    :host { display: block; }
    .pic { overflow-x: auto; }
    .pic svg { display: block; max-width: none; }
    .note { font-size: 12.5px; color: var(--muted); margin-top: 4px; }
  `;
  declare source: string;
  declare svg: string;
  declare project: string;
  declare note: string;
  private width = 0;
  private ro?: ResizeObserver;
  private seq = 0;

  constructor() {
    super();
    this.source = "";
    this.svg = "";
    this.project = "";
    this.note = "";
  }

  connectedCallback() {
    super.connectedCallback();
    this.ro = new ResizeObserver(() => {
      const w = Math.floor(this.getBoundingClientRect().width);
      if (w > 0 && Math.abs(w - this.width) > 40) {
        this.width = w;
        void this.redraw();
      }
    });
    this.ro.observe(this);
  }

  disconnectedCallback() {
    super.disconnectedCallback();
    this.ro?.disconnect();
  }

  /** The source with the board's cards as items of an own section. */
  private async withCards(): Promise<string> {
    const board = /^\s*board\s*:\s*(\S+)\s*$/im.exec(this.source)?.[1];
    if (!board) return this.source;
    let cards: Task[] = [];
    try {
      cards = await this.sdk.tasks.list(board);
    } catch {
      this.note = `Board "${board}" could not be loaded.`;
      return this.source;
    }
    const due = cards.filter((c) => /^\d{4}-\d{2}-\d{2}$/.test(c.due ?? ""));
    if (!due.length) return this.source;
    const clean = (s: string) => s.replace(/[:[\]\n]/g, " ").trim() || "–";
    const lines = due.map((c) => {
      const made = (c.createdAt ?? "").slice(0, 10);
      const start = made && made < c.due! ? made : c.due!;
      return `${clean(c.title)}: ${start}${start === c.due ? "" : `, ${c.due}`}${c.doneAt ? ", done" : ""}`;
    });
    return `${this.source}\n## ${clean(due[0].boardTitle ?? board)}\n${lines.join("\n")}`;
  }

  private async redraw() {
    const n = ++this.seq;
    const src = await this.withCards();
    if (src === this.source && Math.abs(this.width - 720) < 40) return; // the first picture fits
    try {
      const r = await this.sdk.blocks.render("gantt", src, this.width || 720);
      if (n === this.seq && r.svg) this.svg = r.svg;
    } catch {
      /* keep the picture we have */
    }
  }

  updated() {
    // The host cleaned this SVG (no scripts, handlers or outside links).
    const box = this.renderRoot.querySelector(".pic");
    if (box && box.getAttribute("data-svg") !== String(this.svg.length)) {
      box.innerHTML = this.svg;
      box.setAttribute("data-svg", String(this.svg.length));
    }
  }

  render() {
    return html`<div class="pic" role="img" aria-label="Schedule"></div>${this.note ? html`<p class="note">${this.note}</p>` : ""}`;
  }
}

export default definePlugin({
  elements: { "phx-gantt": GanttView },
  blocks: [{ id: "gantt", element: "phx-gantt", icon: "chart-gantt" }],
});
