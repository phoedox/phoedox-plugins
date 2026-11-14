import { css, definePlugin, html, PhxElement } from "@phoedox/sdk";

interface Control { slug: string; name: string; entity_id: string; state?: string }
interface Controls { entry?: string; self?: Control; power?: Control }

const de = (navigator.language || "de").toLowerCase().startsWith("de");
const tr = {
  title: de ? "Home Assistant" : "Home Assistant",
  on: de ? "Einschalten" : "Switch on",
  off: de ? "Ausschalten" : "Switch off",
  restart: de ? "Neu starten" : "Restart",
  state: (s: string) => (de ? { on: "an", off: "aus", unavailable: "nicht erreichbar", unknown: "unbekannt" } : { on: "on", off: "off", unavailable: "unavailable", unknown: "unknown" } as Record<string, string>)[s] ?? s,
  via: (n: string) => (de ? `Strom über ${n}` : `Powered by ${n}`),
  confirmSwitch: (n: string, on: boolean) => (de ? `${n} wirklich ${on ? "einschalten" : "ausschalten"}?` : `Really switch ${n} ${on ? "on" : "off"}?`),
  confirmRestart: (dev: string, plug: string) => de
    ? `${dev} neu starten? ${plug} wird ausgeschaltet und nach 10 Sekunden wieder eingeschaltet. Alles an dieser Steckdose verliert kurz den Strom.`
    : `Restart ${dev}? ${plug} is switched off and on again after 10 seconds. Everything on this outlet loses power briefly.`,
  done: de ? "Erledigt" : "Done",
  restarted: de ? "Strom war 10 s aus und ist wieder an." : "Power was off for 10 s and is back on.",
  busyRestart: de ? "Startet neu … (ca. 10 s)" : "Restarting … (about 10 s)",
};

/** Switches of an entry from Home Assistant: its own switch and the one it is powered by. */
class HaControls extends PhxElement {
  static properties = { entity: { type: String }, c: { state: true }, busy: { state: true }, error: { state: true } };
  static styles = css`
    :host { display: block; }
    .head { display: flex; gap: 8px; align-items: center; font-weight: 600; margin-bottom: 8px; }
    .row { display: flex; gap: 10px; align-items: center; flex-wrap: wrap; padding: 4px 0; }
    .name { min-width: 160px; }
    .state { font-size: 13px; color: var(--muted); }
    .state.on { color: var(--good); } .state.unavailable { color: var(--bad); }
    button { font: inherit; font-size: 13px; padding: 4px 10px; border-radius: 8px; border: 1px solid var(--line); background: var(--surface); color: var(--fg); cursor: pointer; }
    button:disabled { opacity: .6; cursor: progress; }
    @media (hover: hover) and (pointer: fine) { button:not(:disabled):hover { border-color: var(--muted); } }
    .err { color: var(--bad); font-size: 13px; }
  `;
  // Declared so no class field can shadow Lit's accessors, whatever the build's class-field setting.
  declare entity: string;
  declare c?: Controls;
  declare busy: string;
  declare error: string;

  constructor() {
    super();
    this.entity = "";
    this.busy = "";
    this.error = "";
  }

  connectedCallback() {
    super.connectedCallback();
    this.hidden = true;
    void this.load();
  }

  private async load() {
    try {
      this.c = await this.sdk.tool<Controls>("ha.controls", { entity: this.entity });
    } catch {
      this.c = undefined; // plugin not configured or HA unreachable: show nothing
    }
    this.hidden = !this.c?.self && !this.c?.power;
  }

  private async run(what: string, tool: string, input: unknown, message: string) {
    this.busy = what;
    this.error = "";
    try {
      await this.sdk.tool(tool, input);
      this.sdk.ui.toast(message);
      await this.load();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
    } finally {
      this.busy = "";
    }
  }

  private switchRow(c: Control) {
    const on = c.state === "on";
    return html`<div class="row"><span class="name">${c.name}</span>
      <span class="state ${c.state ?? ""}">${tr.state(c.state ?? "unknown")}</span>
      <button ?disabled=${!!this.busy} @click=${() => confirm(tr.confirmSwitch(c.name, !on))
        && this.run("self", "ha.call", { entity_id: c.entity_id, action: on ? "off" : "on" }, tr.done)}>${on ? tr.off : tr.on}</button></div>`;
  }

  render() {
    const c = this.c;
    if (!c || (!c.self && !c.power)) return html``;
    return html`<div class="head">${this.sdk.icons.render("PlugZap", 15)} ${tr.title}</div>
      ${c.self ? this.switchRow(c.self) : ""}
      ${c.power ? html`<div class="row"><span class="name">${tr.via(c.power.name)}</span>
        <span class="state ${c.power.state ?? ""}">${tr.state(c.power.state ?? "unknown")}</span>
        <button ?disabled=${!!this.busy} @click=${() => confirm(tr.confirmRestart(c.entry || this.entity, c.power!.name))
          && this.run("restart", "ha.restart", { entity_id: c.power!.entity_id, seconds: 10 }, tr.restarted)}>
          ${this.busy === "restart" ? tr.busyRestart : tr.restart}</button></div>` : ""}
      ${this.error ? html`<p class="err" role="alert">${this.error}</p>` : ""}`;
  }
}

const kinds = ["device", "smart_plug", "outlet", "light", "router", "repeater", "access_point", "switch", "server", "phone"];

export default definePlugin({
  elements: { "phx-ha-controls": HaControls },
  views: Object.fromEntries(kinds.map((k) => [k, "phx-ha-controls"])),
});
