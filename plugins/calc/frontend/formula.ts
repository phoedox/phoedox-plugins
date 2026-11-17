// Formulas of the calculator block, copied from the core's web/src/formula.ts:
// numbers, text in quotes, names of other values, + - * / ^ & %, comparisons
// and a few functions – without eval. Names are letters, digits and _ (any
// script). Like the plugin's backend (backend/formula.go), a formula is read
// into a tree first, so syntax errors are found before anything is computed.

export class FormulaError extends Error {
  constructor(readonly code: string, readonly detail = "") {
    super(detail ? `${code}: ${detail}` : code);
  }
}

const FUNCS: Record<string, (a: unknown[]) => unknown> = {
  SUM: (a) => nums(a).reduce((x, y) => x + y, 0),
  AVERAGE: (a) => { const n = nums(a); return n.length ? n.reduce((x, y) => x + y, 0) / n.length : 0; },
  MIN: (a) => Math.min(...nums(a)),
  MAX: (a) => Math.max(...nums(a)),
  ROUND: ([x, d]) => { const f = 10 ** (Number(d) || 0); return Math.round(num(x) * f) / f; },
  CEIL: ([x]) => Math.ceil(num(x)),
  FLOOR: ([x]) => Math.floor(num(x)),
  ABS: ([x]) => Math.abs(num(x)),
  SQRT: ([x]) => Math.sqrt(num(x)),
  IF: ([c, a, b]) => (truthy(c) ? a : b ?? ""),
};
FUNCS.AVG = FUNCS.AVERAGE;
export const FORMULA_FUNCTIONS = Object.keys(FUNCS);

const num = (v: unknown): number => {
  if (typeof v === "number") return v;
  if (v === "" || v == null) return 0;
  const n = Number(v);
  if (Number.isNaN(n)) throw new FormulaError("value");
  return n;
};
const nums = (a: unknown[]) => a.filter((v) => v !== "" && v != null && !Number.isNaN(Number(v))).map(Number);
const truthy = (v: unknown) => (typeof v === "string" ? v !== "" && v.toUpperCase() !== "FALSE" : !!v);

export type Tok = { t: "num" | "str" | "name" | "fn" | "op"; v: string | number; src: string };

export function formulaTokens(src: string): Tok[] {
  const out: Tok[] = [];
  let i = 0;
  while (i < src.length) {
    const rest = src.slice(i);
    let m: RegExpExecArray | null;
    if ((m = /^\s+/.exec(rest))) { i += m[0].length; continue; }
    if ((m = /^\d+(?:[.,]\d+)?/.exec(rest))) out.push({ t: "num", v: Number(m[0].replace(",", ".")), src: m[0] });
    else if ((m = /^"([^"]*)"/.exec(rest))) out.push({ t: "str", v: m[1], src: m[0] });
    else if (rest[0] === '"') throw new FormulaError("syntax", 'the closing " is missing');
    else if ((m = /^[\p{L}_][\p{L}\p{N}_]*(?=\s*\()/u.exec(rest)) && FUNCS[m[0].toUpperCase()]) out.push({ t: "fn", v: m[0].toUpperCase(), src: m[0] });
    else if ((m = /^[\p{L}_][\p{L}\p{N}_]*/u.exec(rest))) out.push({ t: "name", v: m[0], src: m[0] });
    else if ((m = /^(<=|>=|<>|[-+*/^&(),;=<>%])/.exec(rest))) out.push({ t: "op", v: m[0] === ";" ? "," : m[0], src: m[0] });
    else throw new FormulaError("syntax", `unexpected "${String.fromCodePoint(rest.codePointAt(0)!)}"`);
    i += m[0].length;
  }
  return out;
}

/** The names a formula uses. */
export function formulaNames(src: string): string[] {
  return [...new Set(formulaTokens(src).filter((t) => t.t === "name").map((t) => String(t.v)))];
}

/** A parsed formula. */
export type FNode =
  | { k: "val"; v: unknown }
  | { k: "name"; name: string }
  | { k: "fn"; name: string; args: FNode[] }
  | { k: "neg" | "pos" | "pct"; x: FNode }
  | { k: "bin"; op: string; l: FNode; r: FNode };

/** Reads a formula into a tree; throws FormulaError("syntax") with a detail. */
export function parseFormula(src: string): FNode {
  const ts = formulaTokens(src);
  let p = 0;
  const isOp = (...ops: string[]) => ts[p]?.t === "op" && ops.includes(String(ts[p].v));
  const close = () => {
    if (p >= ts.length) throw new FormulaError("syntax", "a ) is missing");
    if (!isOp(")")) throw new FormulaError("syntax", `unexpected "${ts[p].src}"`);
    p++;
  };
  const binary = (next: () => FNode, ...ops: string[]) => (): FNode => {
    let l = next();
    while (isOp(...ops)) {
      const op = String(ts[p++].v);
      l = { k: "bin", op, l, r: next() };
    }
    return l;
  };
  function primary(): FNode {
    const t = ts[p++];
    if (!t) throw new FormulaError("syntax", "the formula ends too early");
    if (t.t === "num" || t.t === "str") return { k: "val", v: t.v };
    if (t.t === "name") return { k: "name", name: String(t.v) };
    if (t.t === "fn") {
      if (!isOp("(")) throw new FormulaError("syntax", `${t.v} needs (`);
      p++;
      const args: FNode[] = [];
      if (!isOp(")")) { do args.push(compare()); while (isOp(",") && ++p); }
      close();
      return { k: "fn", name: String(t.v), args };
    }
    if (t.v === "(") { const v = compare(); close(); return v; }
    if (t.v === "-" || t.v === "+") return { k: t.v === "-" ? "neg" : "pos", x: power() };
    throw new FormulaError("syntax", `unexpected "${t.src}"`);
  }
  function percent(): FNode {
    let v = primary();
    while (isOp("%")) { p++; v = { k: "pct", x: v }; }
    return v;
  }
  const power = binary(percent, "^");
  const term = binary(power, "*", "/");
  const sum = binary(term, "+", "-", "&");
  const compare = binary(sum, "=", "<>", "<", ">", "<=", ">=");
  const v = compare();
  if (p < ts.length) throw new FormulaError("syntax", isOp(")") ? "there is a ) without (" : `unexpected "${ts[p].src}"`);
  return v;
}

/** Computes a parsed formula; get(name) gives another value (throws FormulaError("name") if unknown). */
export function evalFormula(n: FNode, get: (name: string) => unknown): unknown {
  const ev = (x: FNode) => evalFormula(x, get);
  switch (n.k) {
    case "val": return n.v;
    case "name": return get(n.name);
    case "fn": return FUNCS[n.name](n.args.map(ev));
    case "neg": return -num(ev(n.x));
    case "pos": return num(ev(n.x));
    case "pct": return num(ev(n.x)) / 100;
  }
  const v = ev(n.l);
  switch (n.op) {
    case "^": { const a = num(v); return a ** num(ev(n.r)); }
    case "*":
    case "/": {
      const r = num(ev(n.r));
      if (n.op === "/" && r === 0) throw new FormulaError("div0");
      return n.op === "*" ? num(v) * r : num(v) / r;
    }
  }
  const r = ev(n.r);
  if (n.op === "&") return `${v}${r}`;
  if (n.op === "+") return num(v) + num(r);
  if (n.op === "-") return num(v) - num(r);
  const [x, y] = typeof v === "number" || typeof r === "number" ? [num(v), num(r)] : [String(v), String(r)];
  return { "=": x === y, "<>": x !== y, "<": x < y, ">": x > y, "<=": x <= y, ">=": x >= y }[n.op];
}

/** The names a parsed formula uses, each once. */
export function nodeNames(n: FNode, out = new Set<string>()): Set<string> {
  if (n.k === "name") out.add(n.name);
  else if (n.k === "fn") n.args.forEach((a) => nodeNames(a, out));
  else if (n.k === "bin") { nodeNames(n.l, out); nodeNames(n.r, out); }
  else if (n.k !== "val") nodeNames(n.x, out);
  return out;
}

/** Parses and computes a formula. */
export function evaluate(src: string, get: (name: string) => unknown): unknown {
  return evalFormula(parseFormula(src), get);
}
