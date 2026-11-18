import { describe, expect, test } from "bun:test";
import {
  fermiPoints,
  fmtClock,
  fruitValue,
  isCardRed,
  kellyDecimal,
  kellyEvenMoney,
  multiplicativeError,
  outcomeClass,
  parseStake,
} from "./logic";

describe("fmtClock", () => {
  test("formats minutes and seconds", () => {
    expect(fmtClock(0)).toBe("0s");
    expect(fmtClock(90_000)).toBe("1:30");
    expect(fmtClock(65_000)).toBe("1:05");
    expect(fmtClock(300_000)).toBe("5:00");
    expect(fmtClock(-5)).toBe("0s");
  });
});

describe("parseStake", () => {
  test("accepts positive finite stakes, rounds to cents", () => {
    expect(parseStake("30", 90)).toBe(30);
    expect(parseStake("10.555", 90)).toBe(10.56);
    expect(parseStake("0.01", 90)).toBe(0.01);
  });
  test("rejects zero, negative, non-numeric", () => {
    expect(Number.isNaN(parseStake("0", 90))).toBeTrue();
    expect(Number.isNaN(parseStake("-5", 90))).toBeTrue();
    expect(Number.isNaN(parseStake("abc", 90))).toBeTrue();
  });
});

describe("fruitValue", () => {
  test("multiplies totals across bags", () => {
    expect(fruitValue([{ apples: 4, oranges: 5 }, { apples: 3, oranges: 6 }], 1)).toBe(77);
  });
  test("applies multiplier and rounds", () => {
    expect(fruitValue([{ apples: 4, oranges: 5 }, { apples: 3, oranges: 6 }], 2)).toBe(154);
    expect(fruitValue([{ apples: 4, oranges: 5 }, { apples: 3, oranges: 6 }], 0.5)).toBe(39);
  });
  test("nullified bag contributes zero", () => {
    expect(fruitValue([{ apples: 0, oranges: 0 }, { apples: 3, oranges: 6 }], 1)).toBe(18);
  });
});

describe("kelly", () => {
  test("even money with pushes uses (pW-pL)/(pW+pL)", () => {
    // Worked example: 4 higher, 8 lower, no pushes -> 1/3.
    expect(kellyEvenMoney(2 / 3, 1 / 3)).toBeCloseTo(1 / 3);
    expect(kellyEvenMoney(0.5, 0.5)).toBe(0);
    expect(kellyEvenMoney(0, 0)).toBe(0);
  });
  test("decimal odds formula matches the dice worked example", () => {
    // P(sum>=8) = 5/12, odds 3.0 -> EV 1/4, Kelly 1/8.
    expect(kellyDecimal(5 / 12, 3.0)).toBeCloseTo(0.125);
    expect(kellyDecimal(0.5, 2.0)).toBe(0);
  });
});

describe("cards and outcomes", () => {
  test("red suits", () => {
    expect(isCardRed("10H")).toBeTrue();
    expect(isCardRed("AS")).toBeFalse();
  });
  test("outcome classes", () => {
    expect(outcomeClass("WIN")).toBe("good");
    expect(outcomeClass("LOSS")).toBe("bad");
    expect(outcomeClass("PUSH")).toBe("warn");
    expect(outcomeClass("MISS")).toBe("");
  });
});

describe("fermi scoring", () => {
  test("multiplicative error is symmetric", () => {
    expect(multiplicativeError(5000, 10000)).toBe(2);
    expect(multiplicativeError(20000, 10000)).toBe(2);
    expect(multiplicativeError(10500, 10000)).toBeCloseTo(1.05);
  });
  test("point table boundaries", () => {
    expect(fermiPoints(1.1)).toBe(10);
    expect(fermiPoints(1.100001)).toBe(8);
    expect(fermiPoints(2)).toBe(4);
    expect(fermiPoints(3)).toBe(2);
    expect(fermiPoints(4)).toBe(0);
  });
});
