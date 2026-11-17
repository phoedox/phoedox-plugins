// The calculator's source format, ported from the core's web/src/blocks/calc.ts
// and kept in step with backend/calc.go: one line per value, "Label (unit) =
// number, formula or @entry.attribute"; "title: …" and # comments.
import { FormulaError, evalFormula, nodeNames, parseFormula, type FNode } from "./formula";

export interface CalcRow {
  line: number;
  label: string;
  name: string;
  unit: string;
  expr: string;
  input: boolean;
  value?: number;
  entity?: { slug: string; attr: string };
  formula?: FNode;
  error?: FormulaError;
}
export interface Calc { title: string; rows: CalcRow[] }
export interface CalcProblem { line: number; message: string }

/** "Cost per year (€)" → name "Cost_per_year", unit "€". */
export function calcName(label: string): { name: string; unit: string } {
  const m = /^(.*?)\s*\(([^)]*)\)\s*$/.exec(label);
  const base = (m ? m[1] : label).trim();
  return { name: base.replace(/[^\p{L}\p{N}_]+/gu, "_").replace(/^_+|_+$/g, ""), unit: m ? m[2].trim() : "" };
}

const NUM = /^-?\d[\d ]*(?:[.,]\d+)?$/;
const ENTITY = /^@([a-z0-9-]+)\.([A-Za-z0-9_]+)$/;

export function parseCalc(src: string): { model?: Calc; problems: CalcProblem[] } {
  const c: Calc = { title: "", rows: [] };
  const problems: CalcProblem[] = [];
  const byName = new Map<string, CalcRow>();
  src.replace(/\r\n/g, "\n").split("\n").forEach((raw, i) => {
    const line = i + 1;
    const l = raw.trim();
    if (!l || l.startsWith("#")) return;
    const kv = /^([A-Za-z]+)\s*:\s*(.*)$/.exec(l);
    if (kv && ["title", "titel"].includes(kv[1].toLowerCase())) { c.title = kv[2].trim(); return; }
    const eq = l.indexOf("=");
    const label = l.slice(0, eq).trim(), expr = l.slice(eq + 1).trim();
    const { name, unit } = calcName(label);
    if (eq <= 0 || !name || !expr) { problems.push({ line, message: `"${l}": a line needs label = value or formula` }); return; }
    if (byName.has(name)) problems.push({ line, message: `the name "${name}" is there twice` });
    const r: CalcRow = { line, label, name, unit, expr, input: false };
    byName.set(name, r);
    c.rows.push(r);
    const ent = ENTITY.exec(expr);
    if (NUM.test(expr)) Object.assign(r, { input: true, value: Number(expr.replace(/ /g, "").replace(",", ".")) });
    else if (ent) Object.assign(r, { input: true, entity: { slug: ent[1], attr: ent[2] } });
    else {
      try {
        r.formula = parseFormula(expr);
      } catch (e) {
        r.error = e instanceof FormulaError ? e : new FormulaError("syntax");
        problems.push({ line, message: `the formula of "${name}" cannot be read: ${r.error.detail || r.error.code}` });
      }
    }
  });
  for (const r of c.rows) {
    if (!r.formula) continue;
    for (const n of nodeNames(r.formula)) {
      if (!byName.has(n)) problems.push({ line: r.line, message: `"${r.name}" uses "${n}", which does not exist (names are labels with _ for spaces, without the unit)` });
    }
  }
  if (!c.rows.length) problems.push({ line: 1, message: "the calculator is empty" });
  if (!problems.length) {
    const r = cycle(c, byName);
    if (r) problems.push({ line: r.line, message: `"${r.name}" depends on itself` });
  }
  problems.sort((a, b) => a.line - b.line);
  return problems.length ? { problems } : { model: c, problems };
}

/** A row whose formula (through others) uses itself. */
function cycle(c: Calc, byName: Map<string, CalcRow>): CalcRow | undefined {
  const state = new Map<CalcRow, 1 | 2>();
  const visit = (r?: CalcRow): CalcRow | undefined => {
    if (!r || state.get(r) === 2) return undefined;
    if (state.get(r) === 1) return r;
    state.set(r, 1);
    for (const n of r.formula ? nodeNames(r.formula) : []) {
      const hit = visit(byName.get(n));
      if (hit) return hit;
    }
    state.set(r, 2);
    return undefined;
  };
  for (const r of c.rows) {
    const hit = visit(r);
    if (hit) return hit;
  }
  return undefined;
}

/** All values: inputs (overridden by the reader) and formulas; errors as FormulaError. */
export function calcValues(c: Calc, inputs: Record<string, number | undefined>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  const rows = new Map(c.rows.map((r) => [r.name, r]));
  const busy = new Set<string>();
  const get = (name: string): unknown => {
    if (name in out) {
      if (out[name] instanceof FormulaError) throw out[name];
      return out[name];
    }
    const r = rows.get(name);
    if (!r) throw new FormulaError("name");
    if (busy.has(name)) throw new FormulaError("cycle");
    busy.add(name);
    try {
      let v: unknown;
      if (r.input) v = inputs[name] ?? r.value ?? 0;
      else if (r.formula) v = evalFormula(r.formula, get);
      else throw r.error ?? new FormulaError("syntax");
      out[name] = v;
      return v;
    } catch (e) {
      out[name] = e instanceof FormulaError ? e : new FormulaError("value");
      throw out[name];
    } finally {
      busy.delete(name);
    }
  };
  for (const r of c.rows) { try { get(r.name); } catch { /* kept in out */ } }
  return out;
}

/** "de" for German locales, else "en". */
export const lang = (loc: string): "de" | "en" => (/^de\b/i.test(loc) ? "de" : "en");

/** A value for reading: numbers in the locale, ✓/✗, #DIV/0, #CYCLE, #ERROR. */
export function formatValue(v: unknown, loc = "en"): string {
  const de = lang(loc) === "de";
  if (v instanceof FormulaError) return v.code === "div0" ? "#DIV/0" : v.code === "cycle" ? (de ? "#ZYKLUS" : "#CYCLE") : de ? "#FEHLER" : "#ERROR";
  if (typeof v === "number") return new Intl.NumberFormat(de ? "de" : "en", { maximumFractionDigits: Math.abs(v) >= 100 ? 0 : 2 }).format(v);
  if (typeof v === "boolean") return v ? "✓" : "✗";
  return String(v ?? "");
}
