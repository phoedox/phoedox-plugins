import { css, definePlugin, html, PhxElement, type Entity } from "@phoedox/sdk";

const statusLabel: Record<string, string> = { online: "Netzbetrieb", batterie: "Batteriebetrieb", batterie_schwach: "Batterie schwach", unbekannt: "Unbekannt" };

/** Live charge, runtime and status on a UPS entry (and as a block). */
class UpsView extends PhxElement {
  static properties = { entity: { type: String }, e: { state: true } };
  static styles = css`
    :host { display: block; }
    .row { display: flex; gap: 18px; align-items: baseline; flex-wrap: wrap; }
    .big { font-size: 28px; font-weight: 700; font-variant-numeric: tabular-nums; }
    .muted { color: var(--muted); font-size: 13px; }
    .bar { height: 8px; border-radius: 99px; background: var(--surface2); overflow: hidden; margin-top: 8px; max-width: 360px; }
    .bar i { display: block; height: 100%; background: var(--good); transition: width 300ms cubic-bezier(0.23, 1, 0.32, 1); }
    .bar i.low { background: var(--bad); }
    @media (prefers-reduced-motion: reduce) { .bar i { transition: none; } }
  `;
  entity = "";
  e?: Entity;
  private stop?: () => void;

  connectedCallback() {
    super.connectedCallback();
    this.stop = this.sdk.graph.watch(this.entity, (e) => (this.e = e));
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    this.stop?.();
  }
  render() {
    const a = this.e?.attrs ?? {};
    if (a.ladung === undefined) return html`<span class="muted">Noch keine Messwerte – das Plugin fragt jede Minute nach.</span>`;
    const charge = Number(a.ladung);
    return html`<div class="row">
        <span class="big">${charge} %</span>
        <span>${a.laufzeit !== undefined ? Number(a.laufzeit).toLocaleString() : "?"} min Laufzeit</span>
        <span>${statusLabel[String(a.status)] ?? a.status}</span>
        ${a.abgefragt ? html`<span class="muted">Stand ${new Date(String(a.abgefragt)).toLocaleTimeString()}</span>` : ""}
      </div>
      <div class="bar" role="meter" aria-valuemin="0" aria-valuemax="100" aria-valuenow=${charge} aria-label="Ladung"><i class=${charge < 30 ? "low" : ""} style="width:${charge}%"></i></div>`;
  }
}

export default definePlugin({
  elements: { "phx-ups-view": UpsView },
  views: { ups: "phx-ups-view" },
  blocks: [{ id: "ups-status", element: "phx-ups-view", icon: "battery-charging" }],
});
