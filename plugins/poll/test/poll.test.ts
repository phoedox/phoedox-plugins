// Run from the repository root: bun test plugins/poll
import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { myChoices, parsePoll, pollKey, share, toggleChoice } from "../frontend/poll";

describe("poll", () => {
  test("question and answers (cases of the core)", () => {
    const { model } = parsePoll("question: Wohin?\n- Zoo\n- Museum\nmultiple: ja\nid: ausflug");
    expect(model).toEqual({ question: "Wohin?", options: ["Zoo", "Museum"], multiple: true, closed: false, results: "after", id: "ausflug" });
    expect(parsePoll("Wohin?\n- Zoo\n- Zoo").problems[0].key).toBe("err_duplicate");
    expect(parsePoll("Wohin?\n- Zoo").problems[0].key).toBe("err_options");
    expect(pollKey(model, "h123")).toBe("ausflug");
  });

  test("flags on their own line", () => {
    const r = parsePoll("multiple\nWas nehmen wir mit?\n- Picknick\n- Ball\nclosed");
    expect(r.model?.question).toBe("Was nehmen wir mit?");
    expect(r.model?.multiple).toBe(true);
    expect(r.model?.closed).toBe(true);
  });

  test("keys, aliases, comments and results", () => {
    const { model } = parsePoll("# a comment\r\nFrage: Wann?\r\n* Morgen\r\n+  Übermorgen  \r\nresults: immer\r\nclosed: nein\r\nMULTIPLE: off");
    expect(model).toEqual({ question: "Wann?", options: ["Morgen", "Übermorgen"], multiple: false, closed: false, results: "always" });
    expect(parsePoll("Q\n- a\n- b\nresults: always").model?.results).toBe("always");
    expect(parsePoll("Q\n- a\n- b\nresults: never").model?.results).toBe("after");
    expect(parsePoll("First\nSecond: line\n- a\n- b").model?.question).toBe("First");
  });

  test("problems with lines and the backend's wording", () => {
    expect(parsePoll("- a\n- b").problems).toEqual([{ line: 1, key: "err_question", message: "the question is missing (question: …)" }]);
    expect(parsePoll("Wohin?\n- Zoo\n\n- Zoo").problems).toEqual([
      { line: 4, key: "err_duplicate", params: { option: "Zoo" }, message: 'the answer "Zoo" is there twice' },
    ]);
    const many = ["Q", ...Array.from({ length: 31 }, (_, i) => `- ${i + 1}`)].join("\n");
    expect(parsePoll(many).problems).toEqual([{ line: 32, key: "err_too_many", message: "at most 30 answers" }]);
    expect(parsePoll("").problems.map((p) => p.key)).toEqual(["err_question", "err_options"]);
  });

  test("the key of the votes", () => {
    const { model } = parsePoll("Q\n- a\n- b\nid: trip-2026");
    expect(pollKey(model, "h1")).toBe("trip-2026");
    expect(pollKey(model, "el42", true)).toBe("el42"); // whiteboards: the element's id, like the core
    expect(pollKey(parsePoll("Q\n- a\n- b\nid: not valid!").model, "h1")).toBe("h1");
    expect(pollKey(parsePoll(`Q\n- a\n- b\nid: ${"x".repeat(41)}`).model, "h1")).toBe("h1");
    expect(pollKey(parsePoll("Q\n- a\n- b").model, "h1")).toBe("h1");
  });

  test("voting and counting", () => {
    const one = parsePoll("Q\n- a\n- b\n- c").model!;
    const many = parsePoll("Q\nmultiple\n- a\n- b\n- c").model!;
    expect(toggleChoice(one, ["a"], "b")).toEqual(["b"]);
    expect(toggleChoice(one, ["a"], "a")).toEqual(["a"]);
    expect(toggleChoice(many, ["a"], "b")).toEqual(["a", "b"]);
    expect(toggleChoice(many, ["a", "b"], "a")).toEqual(["b"]);
    expect(toggleChoice(many, ["a"], "a")).toBeNull();
    expect(myChoices(one, { choices: ["a", "gone", 3] })).toEqual(["a"]);
    expect(myChoices(one, null)).toEqual([]);
    expect(myChoices(one, { choices: "a" })).toEqual([]);
    // One answer each: share of the votes; several each: share of the people.
    expect(share(one, { a: 3, b: 1 }, 4, "a")).toEqual({ n: 3, pct: 75 });
    expect(share(many, { a: 3, b: 1 }, 3, "a")).toEqual({ n: 3, pct: 100 });
    expect(share(one, {}, 0, "c")).toEqual({ n: 0, pct: 0 });
  });

  test("the manifest's example works", () => {
    const m = JSON.parse(readFileSync(new URL("../phoedox-plugin.json", import.meta.url), "utf8"));
    const b = m.contributes.blocks[0];
    expect(parsePoll(b.example).problems).toEqual([]);
    expect(b).toMatchObject({ id: "poll", element: "phx-poll", board: true, live: true, size: { w: 420, h: 300 } });
  });
});
