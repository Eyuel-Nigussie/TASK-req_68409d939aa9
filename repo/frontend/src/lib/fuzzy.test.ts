import { describe, expect, it } from "vitest";
import { fuzzyScore, levenshtein } from "./fuzzy";

describe("levenshtein", () => {
  it("returns 0 for equal strings", () => {
    expect(levenshtein("abc", "abc")).toBe(0);
  });
  it("counts single-char edits", () => {
    expect(levenshtein("kitten", "sitting")).toBe(3);
  });
  it("handles empty inputs", () => {
    expect(levenshtein("", "abc")).toBe(3);
    expect(levenshtein("abc", "")).toBe(3);
  });
});

describe("fuzzyScore", () => {
  it("exact match returns 1", () => {
    expect(fuzzyScore("Jane Doe", "jane doe")).toBe(1);
  });
  it("prefix scores above 0.5", () => {
    expect(fuzzyScore("jane", "jane doe")).toBeGreaterThanOrEqual(0.5);
  });
  it("single-char typo still matches", () => {
    expect(fuzzyScore("jane doo", "jane doe")).toBeGreaterThan(0.3);
  });
  it("unrelated scores near zero", () => {
    expect(fuzzyScore("xylophone", "jane doe")).toBeLessThan(0.1);
  });
  it("empty inputs produce zero", () => {
    expect(fuzzyScore("", "jane")).toBe(0);
    expect(fuzzyScore("jane", "")).toBe(0);
  });
});
