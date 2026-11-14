// Run from the repository root: bun test plugins/labels
import { describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { dataCodewords, encodeQr, qrPath, type Ecc } from "../frontend/qr";

const rows = (m: boolean[][]) => m.map((r) => r.map((d) => (d ? "1" : "0")).join(""));
const sha = (m: boolean[][]) => createHash("sha256").update(rows(m).join("\n")).digest("hex");

const url = "https://phoedox.example.org/e/infra/";

// Reference symbols from segno 1.6.6 (byte mode, no error level boost),
// patched to pad only up to the codeword boundary as ISO/IEC 18004 7.4.10
// says (segno adds a whole zero byte when the stream is already aligned).
// Each symbol was also read back with zxing-cpp.
const reference: [text: string, ecc: Ecc, mask: number | undefined, version: number, wantMask: number, sha256: string][] = [
  ["phx://e/infra/nas-01", "M", 2, 2, 2, "c508da277efc99cd0e93a6c93d8e43ecc242276438743cbf5e510f6d62c579b6"],
  ["HELLO WORLD", "Q", 0, 1, 0, "fd28404b7784f535e9c7b652aa6bf5549351f51ada8296e64a5bc05c6e050601"],
  [url + "nas-01", "M", 5, 3, 5, "55322f4a7f2a71a29fa103a6ee46b33ae1ba079f9364c37808a7305f0796da8d"],
  [url + "x".repeat(60), "L", 7, 5, 7, "f1a7e0df04fc8a5e0a12acb428d226d18b22e849168c28a5bed90c68eeddff0d"],
  [url + "y".repeat(70), "Q", 1, 8, 1, "3da8ccc687c71778584d893b876420366bf42895b1768c6a07ec0cf20232163f"],
  ["z".repeat(150), "H", 3, 12, 3, "2386a276f84122e2786ea5c31877c0b8b1ff0193c02d1d43f0f609d5ab9bdb7a"],
  ["a1".repeat(150), "M", 4, 13, 4, "71f635834d97c232b1e137343b71ff7122e6b165eab5ca9cef40f407ead9e13b"],
  ["b".repeat(1000), "L", 6, 22, 6, "f2bb6a50c84003282c94d6a6f17794ca0069d20fed59f9aee1ba26cb3e1d7f1f"],
  ["c".repeat(2953), "L", 2, 40, 2, "c170b08d20f36fabbb69be86effa5011ef1ebfc1708b3d14bdf5c476e38638e5"],
  ["d".repeat(1273), "H", 0, 40, 0, "88a44a8ad6f602c877b38d277cf24cfff7cf8e6ce6b36c3bbfe4d38555559a09"],
  // Automatic mask: the lowest penalty, as segno chooses.
  ["phx://e/infra/nas-01", "M", undefined, 2, 4, "b4f52881284a2f9083d690617a76a1b236f781edb1911b505ff648a1285fa36c"],
  [url + "core-switch-01", "Q", undefined, 5, 2, "0caec71835ca1743f6f1dab1410b5da1d6aad821d8805811107afb76250bfd90"],
];

describe("encodeQr", () => {
  test("matches reference symbols", () => {
    for (const [text, ecc, mask, version, wantMask, hash] of reference) {
      const q = encodeQr(text, ecc, mask === undefined ? {} : { mask });
      expect([text.slice(0, 20), q.version, q.mask, q.size]).toEqual([text.slice(0, 20), version, wantMask, version * 4 + 17]);
      expect(sha(q.modules)).toBe(hash);
    }
  });

  test("version 2-M symbol module by module", () => {
    const q = encodeQr("phx://e/infra/nas-01", "M", { mask: 2 });
    expect(rows(q.modules)).toEqual([
      "1111111000100100001111111",
      "1000001001110011101000001",
      "1011101010011010001011101",
      "1011101010011101101011101",
      "1011101010010010101011101",
      "1000001010110001101000001",
      "1111111010101010101111111",
      "0000000011100001000000000",
      "1011111001110101001111100",
      "1100010111001010100101110",
      "0100111100111001010101011",
      "0011010010101010100011001",
      "1001111010110000011010100",
      "1010000011101100100101100",
      "1000001001000111111111111",
      "1010000100101100100111001",
      "1001001011001010111111101",
      "0000000010000001100010110",
      "1111111001011000101010111",
      "1000001010110011100011001",
      "1011101011110001111111111",
      "1011101011001110011010111",
      "1011101010000111010000101",
      "1000001001101101101100001",
      "1111111011101010000101111",
    ]);
  });

  test("format information encodes level and mask", () => {
    for (const ecc of ["L", "M", "Q", "H"] as Ecc[]) {
      for (let mask = 0; mask < 8; mask++) {
        const q = encodeQr("phx://e/p/s", ecc, { mask });
        // Bits 14..9 of the format sit in row 8, columns 0–5.
        let bits = 0;
        for (let x = 0; x <= 5; x++) bits = (bits << 1) | (q.modules[8][x] ? 1 : 0);
        const data = (bits ^ (0x5412 >>> 9)) >>> 1; // drop bit 9 (part of the BCH code)
        expect(data).toBe(({ L: 1, M: 0, Q: 3, H: 2 }[ecc] << 3) | mask);
        // The dark module is always set.
        expect(q.modules[q.size - 8][8]).toBe(true);
      }
    }
  });

  test("counts UTF-8 bytes and picks the smallest version", () => {
    expect(encodeQr("a".repeat(14), "M").version).toBe(1);
    expect(encodeQr("a".repeat(15), "M").version).toBe(2);
    // "ü" takes two bytes.
    expect(encodeQr("ü".repeat(7), "M").version).toBe(1);
    expect(encodeQr("ü".repeat(8), "M").version).toBe(2);
    expect(encodeQr("a", "L", { minVersion: 3 }).version).toBe(3);
    expect(dataCodewords(1, "L")).toBe(19);
    expect(dataCodewords(40, "H")).toBe(1276);
  });

  test("rejects what does not fit", () => {
    expect(() => encodeQr("c".repeat(2954), "L")).toThrow(RangeError);
    expect(() => encodeQr("c".repeat(100), "H", { maxVersion: 3 })).toThrow(RangeError);
    expect(() => encodeQr("x", "M", { mask: 8 })).toThrow(RangeError);
  });

  test("path has one square per dark module", () => {
    const q = encodeQr("HELLO WORLD", "Q", { mask: 0 });
    const dark = q.modules.flat().filter(Boolean).length;
    const path = qrPath(q, 2);
    expect(path.split("z").length - 1).toBe(dark);
    expect(path.startsWith("M2 2h1v1h-1z")).toBe(true); // top-left finder corner
  });
});
