// The quiz's source format, ported from the core's web/src/blocks/quiz.ts and
// kept in step with backend/quiz.go: "? question", then "+ right" / "- wrong"
// answers or "= answer" for a flashcard, "> explanation" shown after answering;
// "title:" and "id:" before the first question, # comments.

export interface QuizOption { text: string; right: boolean }
export interface QuizQuestion { q: string; options: QuizOption[]; answer?: string; explain?: string }
export interface Quiz { title: string; id?: string; questions: QuizQuestion[] }
export interface QuizProblem { line: number; message: string }

/** A reader's answer per question index: the chosen option, or whether they knew a flashcard. */
export type Answer = number | "known" | "unknown";
export type Answers = Record<string, Answer>;
/** The saved state of a reader, the same shape the core's quiz kept (null when nothing is answered). */
export interface QuizState { answers?: Answers }

const KEYS = ["title", "titel", "id"];

/** Reads a quiz; problems name their line (from 1) in the backend's words. With problems there is no model. */
export function parseQuiz(src: string): { model?: Quiz; problems: QuizProblem[] } {
  const z: Quiz = { title: "", questions: [] };
  const problems: QuizProblem[] = [];
  const lines: number[] = [];
  let cur: QuizQuestion | undefined;
  src.replace(/\r\n/g, "\n").split("\n").forEach((raw, i) => {
    const line = i + 1;
    const l = raw.trim();
    if (!l || l.startsWith("#")) return;
    if (!cur) {
      const kv = /^([A-Za-z]+)\s*:\s*(.*)$/.exec(l);
      const k = kv?.[1].toLowerCase() ?? "";
      if (kv && KEYS.includes(k)) {
        if (k === "id") z.id = kv[2].trim();
        else z.title = kv[2].trim();
        return;
      }
    }
    const m = /^([?+\-=>])\s*(.*)$/.exec(l);
    if (!m) {
      problems.push({ line, message: `${JSON.stringify(l)}: lines start with ?, +, -, = or >` });
      return;
    }
    const [, mark, text] = m;
    if (mark === "?") {
      cur = { q: text, options: [] };
      z.questions.push(cur);
      lines.push(line);
      return;
    }
    if (!cur) {
      problems.push({ line, message: `${JSON.stringify(l)} comes before the first question (? …)` });
      return;
    }
    if (mark === "+" || mark === "-") cur.options.push({ text, right: mark === "+" });
    else if (mark === "=") cur.answer = text;
    else cur.explain = cur.explain ? `${cur.explain} ${text}` : text;
  });
  if (!z.questions.length) problems.push({ line: 1, message: "a question is missing (? …)" });
  z.questions.forEach((q, i) => {
    if (!q.options.length && !q.answer) problems.push({ line: lines[i], message: `question ${i + 1} needs answers (+ right, - wrong) or a solution (= …)` });
    else if (q.options.length && !q.options.some((o) => o.right)) problems.push({ line: lines[i], message: `question ${i + 1} has no right answer (+ …)` });
  });
  problems.sort((a, b) => a.line - b.line);
  return problems.length ? { problems } : { model: z, problems };
}

/** The core's hash of a block's source (FNV-1a, base 36 with "h"), the host's stateKey. */
export function hashText(s: string): string {
  let h = 2166136261;
  for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 16777619);
  return "h" + (h >>> 0).toString(36);
}

/**
 * The key of the readers' answers, as the core chose it: the quiz's own id when it is
 * valid (1–40 of A–Z a–z 0–9 -), else the host's stateKey (a hash of the trimmed source).
 */
export function quizKey(z: Quiz | undefined, source: string, stateKey: string): string {
  const id = z?.id;
  return id && /^[A-Za-z0-9-]{1,40}$/.test(id) ? id : stateKey || hashText(source.trim());
}

/** The saved answers, or none when the state is not a quiz's. */
export function answersOf(state: unknown): Answers {
  const a = (state as QuizState | null)?.answers;
  if (!a || typeof a !== "object" || Array.isArray(a)) return {};
  const out: Answers = {};
  for (const [k, v] of Object.entries(a)) if (typeof v === "number" || v === "known" || v === "unknown") out[k] = v;
  return out;
}

/** How many questions the reader got right (a right option, or a flashcard they knew). */
export function quizScore(z: Quiz, answers: Answers): { right: number; done: number } {
  let right = 0, done = 0;
  z.questions.forEach((q, i) => {
    const a = answers[i];
    if (a === undefined) return;
    done++;
    if (a === "known" || (typeof a === "number" && q.options[a]?.right)) right++;
  });
  return { right, done };
}

/** "de" for German locales, else "en". */
export const lang = (loc: string): "de" | "en" => (/^de\b/i.test(loc) ? "de" : "en");
