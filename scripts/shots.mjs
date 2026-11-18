// Screenshots of a plugin in use (concept "Plugin logos and screenshots",
// ADR-0090): node scripts/shots.mjs <plugin…|all>
//
// For each plugin with a plugins/<name>/shots.json it starts a fresh
// Phoedox (built from ../phoedox, or PHOEDOX_DIR) with an empty SQLite data
// folder, installs the packed plugin, starts the plugin's fake API
// (testdata/fake, see internal/fakeapi), runs the setup and takes the shots
// with Playwright: 1280×800 at device scale 2, light theme, en-US, clock
// fixed at 2026-06-15 10:00 Europe/Berlin. It writes
// plugins/<name>/screenshots/<shot>.webp, the manifest's "screenshots" and
// the README's image line.
//
// shots.json:
//   {
//     "secrets": {"url": "${fake}", "token": "demo"},        // project settings of the plugin
//     "hosts":   ["api.hetzner.cloud"],                       // fixed hosts sent to the fake API
//     "setup":   [{"method": "POST", "path": "/projects/${project}/documents", "body": {…}, "as": "doc"}],
//     "wait":    {"path": "/projects/${project}/entities", "min": 8, "timeout": 30000},
//     "shots":   [{"name": "main", "path": "/p/${project}/graph", "caption": "…",
//                  "selector": "phx-plugin-block", "actions": [{"click": "text=Run"}], "wait": 1500}]
//   }
// ${project} is the demo project's slug, ${fake} the fake API's address,
// ${rsaKey} a fresh RSA key (PKCS #8 PEM, JSON-escaped) for services that
// sign tokens, made for each run so no key is stored in the repository, and
// ${<as>.<field>} a field of an earlier setup answer.
import { spawn, execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { createRequire } from "node:module";
import { generateKeyPairSync } from "node:crypto";

const require = createRequire(import.meta.url);
const { chromium } = require("playwright");

const repo = path.resolve(path.dirname(new URL(import.meta.url).pathname), "..");
const phoedox = path.resolve(process.env.PHOEDOX_DIR ?? path.join(repo, "..", "phoedox"));
const port = Number(process.env.SHOTS_PORT ?? 18190);
const base = `http://127.0.0.1:${port}`;
const admin = { email: "alex@example.org", password: "shots-demo-password-1" };
const clock = new Date("2026-06-15T10:00:00+02:00");
const W = 1280, H = 800, SCALE = 2;

const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "phx-shots-"));
const bin = path.join(tmp, "bin");

function sh(cmd, args, opts = {}) {
  return execFileSync(cmd, args, { stdio: ["ignore", "pipe", "inherit"], encoding: "utf8", ...opts }).trim();
}

function build() {
  fs.mkdirSync(bin, { recursive: true });
  if (!fs.existsSync(path.join(phoedox, "web", "dist", "index.html"))) sh("bun", ["run", "build"], { cwd: path.join(phoedox, "web") });
  sh("go", ["build", "-o", path.join(bin, "phoedox"), "./cmd/phoedox"], { cwd: phoedox });
  sh("go", ["build", "-o", path.join(bin, "phx"), "./cmd/phx"], { cwd: phoedox });
  sh("go", ["build", "-o", path.join(bin, "fakeapi"), "./tools/fakeapi"], { cwd: repo });
}

async function waitFor(fn, timeout = 30000, what = "condition") {
  const end = Date.now() + timeout;
  for (;;) {
    try { if (await fn()) return; } catch { /* not yet */ }
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
    await new Promise((r) => setTimeout(r, 300));
  }
}

async function startServer(fetchMap = "") {
  const data = fs.mkdtempSync(path.join(tmp, "data-"));
  const env = { ...process.env, PHOEDOX_PLUGINS_FETCH_MAP: fetchMap, PHOEDOX_FAKE_NOW: clock.toISOString(), PHOEDOX_DATA_DIR: data, PHOEDOX_LISTEN: `127.0.0.1:${port}`, PHOEDOX_PUBLIC_URL: base,
    PHOEDOX_PLUGINS_ALLOW_UNSIGNED: "true", PHOEDOX_BACKUP_HOURS: "0", PHOEDOX_INSECURE_COOKIES: "true", PHOEDOX_DEFAULT_LOCALE: "en" };
  sh(path.join(bin, "phx"), ["admin", "create-user", "--email", admin.email, "--name", "Alex Doe", "--admin", "--password", admin.password], { env });
  const proc = spawn(path.join(bin, "phoedox"), [], { env, stdio: ["ignore", "ignore", "pipe"] });
  let log = "";
  proc.stderr.on("data", (d) => (log += d));
  await waitFor(async () => (await fetch(base + "/healthz")).ok, 30000, "Phoedox").catch((e) => { console.error(log); throw e; });
  return proc;
}

async function startFake(name) {
  const dir = path.join(repo, "plugins", name, "testdata", "fake");
  if (!fs.existsSync(path.join(dir, "routes.json"))) return { url: "", stop() {} };
  const proc = spawn(path.join(bin, "fakeapi"), ["-addr", "127.0.0.1:0", dir], { stdio: ["ignore", "pipe", "inherit"] });
  const url = await new Promise((resolve, reject) => {
    proc.stdout.on("data", (d) => { const m = /listening on (\S+)/.exec(String(d)); if (m) resolve(m[1]); });
    proc.on("exit", () => reject(new Error("fakeapi stopped")));
  });
  return { url, stop: () => proc.kill() };
}

function fill(v, vars) {
  if (typeof v === "string") return v.replace(/\$\{([\w.]+)\}/g, (_, k) => k.split(".").reduce((o, p) => o?.[p], vars) ?? "");
  if (Array.isArray(v)) return v.map((x) => fill(x, vars));
  if (v && typeof v === "object") return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, fill(x, vars)]));
  return v;
}

function count(v) {
  if (Array.isArray(v)) return v.length;
  if (v && typeof v === "object") for (const x of Object.values(v)) if (Array.isArray(x)) return x.length;
  return 0;
}

// The PNG of a shot as WebP 2560×1600: element shots are centred on the
// page background with a margin, padded to 16:10.
async function toWebP(browser, png, bg) {
  const page = await browser.newPage();
  const out = await page.evaluate(async ([src, bg, w, h]) => {
    const img = new Image();
    img.src = src;
    await img.decode();
    const c = document.createElement("canvas");
    c.width = w; c.height = h;
    const g = c.getContext("2d");
    g.fillStyle = bg; g.fillRect(0, 0, w, h);
    const s = Math.min(w / img.width, h / img.height, 1);
    g.drawImage(img, (w - img.width * s) / 2, (h - img.height * s) / 2, img.width * s, img.height * s);
    return c.toDataURL("image/webp", 0.85);
  }, [`data:image/png;base64,${png.toString("base64")}`, bg, W * SCALE, H * SCALE]);
  await page.close();
  return Buffer.from(out.split(",")[1], "base64");
}

function writeManifest(dir, shots) {
  const file = path.join(dir, "phoedox-plugin.json");
  let s = fs.readFileSync(file, "utf8");
  const list = shots.map((x) => `    { "file": "screenshots/${x.name}.webp", "caption": ${JSON.stringify(x.caption)} }`).join(",\n");
  const block = `"screenshots": [\n${list}\n  ]`;
  if (/"screenshots":\s*\[[\s\S]*?\]/.test(s)) s = s.replace(/"screenshots":\s*\[[\s\S]*?\n?\s*\]/, block);
  else s = s.replace(/("category": "[^"]*",\n)/, `$1  ${block},\n`);
  JSON.parse(s);
  fs.writeFileSync(file, s);
}

function writeReadme(dir, name, first) {
  const file = path.join(dir, "README.md");
  if (!fs.existsSync(file)) return;
  let s = fs.readFileSync(file, "utf8");
  const line = `![${first.caption}](screenshots/${first.name}.webp)`;
  if (/^!\[[^\]]*\]\((docs\/screenshot\.png|screenshots\/[^)]+)\)$/m.test(s)) s = s.replace(/^!\[[^\]]*\]\((docs\/screenshot\.png|screenshots\/[^)]+)\)$/m, line);
  else s = s.replace(/^(`[^`]+` · version [^\n]+\n)/m, `$1\n${line}\n`);
  fs.writeFileSync(file, s);
}

async function run(browser, name) {
  const dir = path.join(repo, "plugins", name);
  const plan = JSON.parse(fs.readFileSync(path.join(dir, "shots.json"), "utf8"));
  const manifest = JSON.parse(fs.readFileSync(path.join(dir, "phoedox-plugin.json"), "utf8"));
  const pkg = sh(path.join(repo, "scripts", "pack.sh"), [name, path.join(tmp, "dist")], { cwd: repo }).split("\n").pop();
  const fake = await startFake(name);
  // Cloud plugins call fixed hosts: Phoedox sends those to the fake API
  // (PHOEDOX_PLUGINS_FETCH_MAP, development only).
  const server = await startServer((plan.hosts ?? []).map((h) => `${h}=${fake.url}`).join(","));
  const ctx = await browser.newContext({ viewport: { width: W, height: H }, deviceScaleFactor: SCALE, locale: "en-US", timezoneId: "Europe/Berlin", colorScheme: "light", baseURL: base });
  try {
    const page = await ctx.newPage();
    await page.clock.setFixedTime(clock);
    await page.goto("/login");
    await page.fill("input[type=email]", admin.email);
    await page.fill("input[type=password]", admin.password);
    await page.keyboard.press("Enter");
    await page.waitForURL((u) => !u.pathname.startsWith("/login"));
    await waitFor(async () => (await ctx.request.get("/api/v1/me", { headers: { "X-Phoedox": "1" } })).ok(), 10000, "sign-in");
    const api = async (method, p, body, raw) => {
      const res = await ctx.request.fetch("/api/v1" + p, { method, headers: { "X-Phoedox": "1", "Content-Type": raw ? "application/octet-stream" : "application/json" }, data: raw ?? (body === undefined ? undefined : JSON.stringify(body)) });
      const text = await res.text();
      if (!res.ok()) throw new Error(`${method} ${p}: ${res.status()} ${text}`);
      return text ? JSON.parse(text) : null;
    };
    await api("POST", "/admin/plugins", undefined, fs.readFileSync(pkg));
    await api("PATCH", `/admin/plugins/${manifest.id}`, { enabled: true });
    const project = (await api("POST", "/projects", { name: "Maple Street 12", description: "Home of the Doe family: house, network and homelab." })).slug;
    await api("PUT", `/projects/${project}/plugins/${manifest.id}`, { enabled: true });
    const pem = generateKeyPairSync("rsa", { modulusLength: 2048 }).privateKey.export({ type: "pkcs8", format: "pem" });
    const vars = { project, fake: fake.url, rsaKey: JSON.stringify(pem).slice(1, -1) };
    if (plan.secrets) await api("PUT", `/projects/${project}/plugins/${manifest.id}/secrets`, fill(plan.secrets, vars));
    for (const step of plan.setup ?? []) {
      const out = await api(step.method ?? "POST", fill(step.path, vars), fill(step.body, vars));
      if (step.as) vars[step.as] = out;
    }
    if (plan.wait?.path) {
      await waitFor(async () => count(await api("GET", fill(plan.wait.path, vars))) >= (plan.wait.min ?? 1), plan.wait.timeout ?? 30000, plan.wait.path).catch(async (e) => {
        const logs = await api("GET", `/admin/plugins/${manifest.id}/logs`).catch(() => []);
        console.error(`${name}: plugin log:\n` + (logs ?? []).map((l) => `  ${l.level} ${l.msg}`).join("\n"));
        throw e;
      });
    }
    else if (plan.wait?.ms) await page.waitForTimeout(plan.wait.ms);

    fs.mkdirSync(path.join(dir, "screenshots"), { recursive: true });
    for (const shot of plan.shots) {
      await page.goto(fill(shot.path, vars));
      await page.addStyleTag({ content: "phx-toasts{display:none!important} *{caret-color:transparent!important}" });
      await page.waitForLoadState("networkidle").catch(() => {});
      for (const a of shot.actions ?? []) {
        if (a.click) await page.locator(a.click).first().click();
        if (a.fill) await page.locator(a.fill[0]).first().fill(fill(a.fill[1], vars));
        if (a.press) await page.keyboard.press(a.press);
        if (a.wait) await page.waitForTimeout(a.wait);
      }
      await page.waitForTimeout(shot.wait ?? 1200);
      await page.mouse.move(0, H - 1);
      const bg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
      let png;
      if (shot.selector) {
        // The element with a margin, widened to 16:10, cut from the whole
        // page (not only the visible part), then scaled into the frame.
        const el = page.locator(shot.selector).first();
        await el.scrollIntoViewIfNeeded();
        const b = await el.evaluate((n) => { const r = n.getBoundingClientRect(); return { x: r.x + scrollX, y: r.y + scrollY, width: r.width, height: r.height }; });
        const doc = await page.evaluate(() => ({ w: document.documentElement.scrollWidth, h: document.documentElement.scrollHeight }));
        const m = 48;
        let w = b.width + 2 * m, h = b.height + 2 * m;
        if (w / h > W / H) h = (w * H) / W; else w = (h * W) / H;
        w = Math.min(w, doc.w); h = Math.min(h, doc.h);
        const x = Math.min(Math.max(0, b.x + b.width / 2 - w / 2), doc.w - w), y = Math.min(Math.max(0, b.y + b.height / 2 - h / 2), doc.h - h);
        png = await page.screenshot({ fullPage: true, clip: { x, y, width: w, height: h } });
      } else png = await page.screenshot();
      const webp = await toWebP(browser, png, bg);
      if (webp.length > 2 << 20) throw new Error(`${name}/${shot.name}: larger than 2 MB`);
      fs.writeFileSync(path.join(dir, "screenshots", `${shot.name}.webp`), webp);
      console.log(`${name}: screenshots/${shot.name}.webp (${Math.round(webp.length / 1024)} KB)`);
    }
    writeManifest(dir, plan.shots);
    writeReadme(dir, name, plan.shots[0]);
  } finally {
    await ctx.close();
    fake.stop();
    server.kill();
    await new Promise((r) => server.once("exit", r));
  }
}

const args = process.argv.slice(2);
const names = args[0] === "all" || !args.length
  ? fs.readdirSync(path.join(repo, "plugins")).filter((n) => fs.existsSync(path.join(repo, "plugins", n, "shots.json")))
  : args;
build();
const browser = await chromium.launch();
let failed = 0;
for (const n of names) {
  try { await run(browser, n); } catch (e) { failed++; console.error(`${n}: ${e.message}`); }
}
await browser.close();
fs.rmSync(tmp, { recursive: true, force: true });
process.exit(failed ? 1 : 0);
