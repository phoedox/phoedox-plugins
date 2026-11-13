import { css, definePlugin, html, PhxElement } from "@phoedox/sdk";

/** Shows charge and runtime of a UPS entity, live via graph.watch. */
class UpsBlock extends PhxElement {
  static properties = { entity: { type: String }, state: { state: true } };
  static styles = css`.v { font-family: var(--font-mono); }`;
  entity = "";
  state?: Record<string, unknown>;
  private stop?: () => void;

  connectedCallback() {
    super.connectedCallback();
    this.stop = this.sdk.graph.watch(this.entity, (e) => (this.state = e.attrs));
  }
  disconnectedCallback() {
    super.disconnectedCallback();
    this.stop?.();
  }
  render() {
    if (!this.state) return html`…`;
    return html`<span class="v">${this.state.ladung} % · ${this.state.laufzeit} min</span>`;
  }
}

export default definePlugin({
  elements: { "phx-ups-block": UpsBlock },
  blocks: [{ id: "ups-status", element: "phx-ups-block", icon: "battery-charging" }],
});
