// Run from the repository root: bun test plugins/quiz
import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { answersOf, hashText, lang, parseQuiz, quizKey, quizScore } from "../frontend/quiz";

describe("quiz", () => {
  test("choices, flashcards and explanations (cases of the core)", () => {
    const { model, problems } = parseQuiz("title: Netz\n? Router?\n- 1.1\n+ 10.1\n> Seit März.\n\n? WLAN?\n= Gartenhaus");
    expect(problems).toEqual([]);
    expect(model!.title).toBe("Netz");
    expect(model!.questions).toEqual([
      { q: "Router?", options: [{ text: "1.1", right: false }, { text: "10.1", right: true }], explain: "Seit März." },
      { q: "WLAN?", options: [], answer: "Gartenhaus" },
    ]);
    expect(parseQuiz("? A\n- x\n- y").problems[0]).toEqual({ line: 1, message: "question 1 has no right answer (+ …)" });
    expect(parseQuiz("? A").problems[0]).toEqual({ line: 1, message: "question 1 needs answers (+ right, - wrong) or a solution (= …)" });
  });

  test("keys, comments, explanations joined, problems with lines", () => {
    const { model } = parseQuiz("# c\r\nTitel: Netz\r\nid: netz-1\r\n?Router?\r\n+10.1\r\n> one\r\n> two\r\n? WLAN\r\n= A\r\n= B");
    expect(model).toEqual({ title: "Netz", id: "netz-1", questions: [
      { q: "Router?", options: [{ text: "10.1", right: true }], explain: "one two" },
      { q: "WLAN", options: [], answer: "B" },
    ] });
    expect(parseQuiz("title: Q\n# c").problems).toEqual([{ line: 1, message: "a question is missing (? …)" }]);
    expect(parseQuiz("? A\n+ x\nplain text").problems).toEqual([{ line: 3, message: '"plain text": lines start with ?, +, -, = or >' }]);
    expect(parseQuiz("title: T\n+ early\n? A\n+ x").problems).toEqual([{ line: 2, message: '"+ early" comes before the first question (? …)' }]);
    expect(parseQuiz("? A\n+ x\ntitle: late").problems[0].line).toBe(3);
    expect(parseQuiz("? A\n+ x\n\n? B\n> why").problems).toEqual([{ line: 4, message: "question 2 needs answers (+ right, - wrong) or a solution (= …)" }]);
  });

  test("the state key and shape of the core", () => {
    const src = "? Q\n+ a";
    expect(hashText(src)).toMatch(/^h[0-9a-z]+$/);
    expect(quizKey(parseQuiz(src).model, `\n${src}\n`, "")).toBe(hashText(src));
    expect(quizKey(parseQuiz(src).model, src, "hfromhost")).toBe("hfromhost");
    expect(quizKey(parseQuiz(`id: net-quiz\n${src}`).model, src, "hfromhost")).toBe("net-quiz");
    expect(quizKey(parseQuiz(`id: not valid!\n${src}`).model, src, "hfromhost")).toBe("hfromhost");
    expect(answersOf({ answers: { 0: 1, 1: "known", 2: "unknown", 3: "x" } })).toEqual({ 0: 1, 1: "known", 2: "unknown" });
    expect(answersOf(null)).toEqual({});
    expect(answersOf({ answers: [1] })).toEqual({});
    const z = parseQuiz("? A\n- x\n+ y\n? B\n= b\n? C\n+ c").model!;
    expect(quizScore(z, {})).toEqual({ right: 0, done: 0 });
    expect(quizScore(z, { 0: 1, 1: "known" })).toEqual({ right: 2, done: 2 });
    expect(quizScore(z, { 0: 0, 1: "unknown", 2: 0 })).toEqual({ right: 1, done: 3 });
    expect(lang("de-DE")).toBe("de");
    expect(lang("fr")).toBe("en");
  });

  test("the manifest's example works and the quiz stays off whiteboards", () => {
    const m = JSON.parse(readFileSync(new URL("../phoedox-plugin.json", import.meta.url), "utf8"));
    const b = m.contributes.blocks[0];
    expect(b.board).toBeUndefined();
    const { model, problems } = parseQuiz(b.example);
    expect(problems).toEqual([]);
    expect(model!.questions.map((q) => q.answer ?? q.options.findIndex((o) => o.right))).toEqual([1, "Garden house"]);
  });
});
