// The poll's source format, ported from the core's web/src/blocks/poll.ts and
// kept in step with backend/poll.go: a question ("question: …" or the first
// plain line), answers as a list ("- …"), "multiple", "closed", "results" and
// "id" as keys (or the flags multiple/closed on their own line); # comments.

export interface Poll { question: string; options: string[]; multiple: boolean; closed: boolean; results: "always" | "after"; id?: string }

export type ProblemKey = "err_question" | "err_options" | "err_too_many" | "err_duplicate";
/** A finding in the source: the line (from 1), a text key for the view and the backend's English message. */
export interface PollProblem { line: number; key: ProblemKey; params?: Record<string, string | number>; message: string }

/** The most answers a poll may have. */
export const MAX_OPTIONS = 30;

const KEYS = ["question", "frage", "multiple", "closed", "results", "id"];

const yes = (v: string) => /^(true|yes|ja|1|on)$/i.test(v.trim());

/** Reads a poll; without a model when the source has problems. */
export function parsePoll(src: string): { model?: Poll; problems: PollProblem[] } {
  const p: Poll = { question: "", options: [], multiple: false, closed: false, results: "after" };
  const lines: number[] = []; // line of each answer
  src.replace(/\r\n/g, "\n").split("\n").forEach((raw, i) => {
    const l = raw.trim();
    if (!l || l.startsWith("#")) return;
    const kv = /^([A-Za-z]+)\s*:\s*(.*)$/.exec(l);
    const k = kv?.[1].toLowerCase() ?? "";
    if (kv && KEYS.includes(k)) {
      const v = kv[2].trim();
      if (k === "question" || k === "frage") p.question = v;
      else if (k === "multiple") p.multiple = yes(v);
      else if (k === "closed") p.closed = yes(v);
      else if (k === "results") p.results = /always|immer/i.test(v) ? "always" : "after";
      else if (k === "id") p.id = v;
      return;
    }
    // A flag on its own line: "multiple" or "closed".
    if (/^multiple$/i.test(l)) { p.multiple = true; return; }
    if (/^closed$/i.test(l)) { p.closed = true; return; }
    const opt = /^[-*+]\s+(.+)$/.exec(l);
    if (opt) {
      p.options.push(opt[1].trim());
      lines.push(i + 1);
    } else if (!p.question) p.question = l;
  });
  const problems: PollProblem[] = [];
  if (!p.question) problems.push({ line: 1, key: "err_question", message: "the question is missing (question: …)" });
  if (p.options.length < 2) problems.push({ line: 1, key: "err_options", message: "a poll needs at least two answers (- …)" });
  if (p.options.length > MAX_OPTIONS) problems.push({ line: lines[MAX_OPTIONS], key: "err_too_many", message: `at most ${MAX_OPTIONS} answers` });
  const dup = p.options.findIndex((o, i) => p.options.indexOf(o) !== i);
  if (dup >= 0) {
    const option = p.options[dup];
    problems.push({ line: lines[dup], key: "err_duplicate", params: { option }, message: `the answer ${JSON.stringify(option)} is there twice` });
  }
  problems.sort((a, b) => a.line - b.line);
  return problems.length ? { problems } : { model: p, problems };
}

/**
 * The key of the poll's votes, the same as the core's: on a whiteboard the
 * element's id (the host's stateKey), on a page the poll's own id if it is a
 * valid one (1–40 of A–Z a–z 0–9 -), else the host's key (a hash of the
 * trimmed source).
 */
export function pollKey(p: Poll | undefined, stateKey: string, board = false): string {
  const id = p?.id;
  return !board && id && /^[A-Za-z0-9-]{1,40}$/.test(id) ? id : stateKey;
}

/** The reader's choices from a saved state ({choices: [...]}), only those that are still answers. */
export function myChoices(p: Poll, mine: { choices?: unknown } | null | undefined): string[] {
  const c = mine?.choices;
  return Array.isArray(c) ? c.filter((x): x is string => typeof x === "string" && p.options.includes(x)) : [];
}

/** The choices after clicking an answer (null: no vote left, the state is removed). */
export function toggleChoice(p: Poll, mine: string[], option: string): string[] | null {
  const next = p.multiple ? (mine.includes(option) ? mine.filter((x) => x !== option) : [...mine, option]) : [option];
  return next.length ? next : null;
}

/**
 * An answer's number of votes and its share in percent. With several answers
 * each the share is of the people who voted; with one answer each, of the votes.
 */
export function share(p: Poll, votes: Record<string, number>, voters: number, option: string): { n: number; pct: number } {
  const total = Object.values(votes).reduce((a, b) => a + b, 0);
  const base = p.multiple ? voters : total;
  const n = votes[option] ?? 0;
  return { n, pct: base ? Math.round((n / base) * 100) : 0 };
}

export const lang = (loc: string): "de" | "en" => (/^de\b/i.test(loc) ? "de" : "en");
