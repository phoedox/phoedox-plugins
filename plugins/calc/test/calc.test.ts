// Run from the repository root: bun test plugins/calc
import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { calcName, calcValues, formatValue, parseCalc } from "../frontend/calc";
import { FormulaError, evaluate, formulaNames } from "../frontend/formula";

describe("formulas", () => {
  test("arithmetic, names, functions, percent (cases of the core)", () => {
    const v: Record<string, number> = { Leistung: 25, Stunden_pro_Tag: 24, Preis: 0.32 };
    const get = (n: string) => { if (!(n in v)) throw new FormulaError("name"); return v[n]; };
    expect(evaluate("Leistung * Stunden_pro_Tag * 365 / 1000 * Preis", get)).toBeCloseTo(70.08);
    expect(evaluate("ROUND(10/3; 1)", get)).toBe(3.3);
    expect(evaluate("19%*100", get)).toBe(19);
    expect(evaluate('IF(Leistung > 20, "viel", "wenig")', get)).toBe("viel");
    expect(evaluate("2,5 * 2", get)).toBe(5);
    expect(evaluate("2 ^ 3 ^ 2", get)).toBe(64);
    expect(evaluate("-2 ^ 2", get)).toBe(-4);
    expect(evaluate('"a" & 1 & "b"', get)).toBe("a1b");
    expect(() => evaluate("1/0", get)).toThrow("div0");
    expect(() => evaluate("Unbekannt + 1", get)).toThrow("name");
    expect(() => evaluate("(1 + 2", get)).toThrow(") is missing");
    expect(() => evaluate('"(" 1 ")"', get)).toThrow("syntax");
    expect(formulaNames("Größe * Anzahl + SUM(a, b)")).toEqual(["Größe", "Anzahl", "a", "b"]);
  });
});

describe("calculator", () => {
  test("inputs, formulas, units, problems with lines", () => {
    expect(calcName("Kosten pro Jahr (€)")).toEqual({ name: "Kosten_pro_Jahr", unit: "€" });
    const { model, problems } = parseCalc("title: Strom\nLeistung (W) = 25\nPreis (€/kWh) = 0,32\nKosten pro Jahr (€) = Leistung * 24 * 365 / 1000 * Preis\nPro Monat (€) = Kosten_pro_Jahr / 12\nNAS (W) = @nas.power_w");
    expect(problems).toEqual([]);
    expect(model!.rows.find((r) => r.name === "NAS")!.entity).toEqual({ slug: "nas", attr: "power_w" });
    expect(calcValues(model!, {}).Kosten_pro_Jahr).toBeCloseTo(70.08);
    expect(calcValues(model!, { Leistung: 50 }).Pro_Monat).toBeCloseTo(11.68);
    expect(formatValue(1234.567, "de")).toBe("1.235");
    expect(formatValue(5.678, "de")).toBe("5,68");
    expect(formatValue(5.678, "en")).toBe("5.68");
    expect(formatValue(new FormulaError("cycle"), "en")).toBe("#CYCLE");
    expect(parseCalc("A = B + 1\nB = A * 2").problems[0]).toEqual({ line: 1, message: '"A" depends on itself' });
    expect(parseCalc("A = 1\nB = C + 1").problems[0]).toEqual({ line: 2, message: '"B" uses "C", which does not exist (names are labels with _ for spaces, without the unit)' });
    expect(parseCalc("nur Text").problems[0].message).toContain("a line needs label = value or formula");
    expect(parseCalc("A = 1\nA = 2").problems[0]).toEqual({ line: 2, message: 'the name "A" is there twice' });
    expect(parseCalc("A = 1\nB = A $ 2").problems[0].message).toBe('the formula of "B" cannot be read: unexpected "$"');
  });

  test("the manifest's example works", () => {
    const m = JSON.parse(readFileSync(new URL("../phoedox-plugin.json", import.meta.url), "utf8"));
    const { model, problems } = parseCalc(m.contributes.blocks[0].example);
    expect(problems).toEqual([]);
    expect(calcValues(model!, {}).Cost_per_month).toBeCloseTo(5.84);
  });
});
