import { describe, expect, test } from "bun:test";
import { clearGameResults, loadGameResults, recordFinishedResult, summarizeResult, type ScoreEntry } from "./scores";

// Map-backed storage stand-in; real localStorage lives in the browser.
function fakeStore() {
  const map = new Map<string, string>();
  return {
    getItem: (k: string) => (map.has(k) ? map.get(k)! : null),
    setItem: (k: string, v: string) => void map.set(k, v),
    removeItem: (k: string) => void map.delete(k),
  };
}

const base = { slug: "fruit", sessionId: "s-1", me: "p1" };
const fruitResults = { players: [{ player: "p1", raw_profit: 120, accuracy: 0.8, final_score: 96, rank: 1 }] };

describe("recordFinishedResult", () => {
  test("never records an unfinished session", () => {
    const s = fakeStore();
    expect(recordFinishedResult({ ...base, finished: false, results: fruitResults }, s)).toBeNull();
    expect(loadGameResults(undefined, s)).toHaveLength(0);
    expect(recordFinishedResult({ ...base, finished: false, results: undefined }, s)).toBeNull();
  });

  test("records a completed session with per-game summary", () => {
    const s = fakeStore();
    const entry = recordFinishedResult({ ...base, finished: true, results: fruitResults }, s);
    expect(entry).not.toBeNull();
    expect(entry!.game).toBe("fruit");
    expect(entry!.score).toBe(96);
    expect(entry!.label).toBe("final score");
    expect(entry!.detail).toContain("80%");
    const all = loadGameResults(undefined, s) as ScoreEntry[];
    expect(all).toHaveLength(1);
    expect(all[0].session).toBe("s-1");
  });

  test("one session never produces two entries", () => {
    const s = fakeStore();
    recordFinishedResult({ ...base, finished: true, results: fruitResults }, s);
    expect(recordFinishedResult({ ...base, finished: true, results: fruitResults }, s)).toBeNull();
    expect(loadGameResults(undefined, s)).toHaveLength(1);
  });

  test("caps the stored history at 100 entries", () => {
    const s = fakeStore();
    for (let i = 1; i <= 120; i++) {
      recordFinishedResult({ slug: "math", sessionId: `s-${i}`, me: "p1", finished: true, results: { score: i } }, s);
    }
    expect(loadGameResults(undefined, s)).toHaveLength(100);
    // Newest first.
    expect(loadGameResults(undefined, s)[0].session).toBe("s-120");
  });

  test("filters by game slug", () => {
    const s = fakeStore();
    recordFinishedResult({ ...base, finished: true, results: fruitResults }, s);
    recordFinishedResult({ slug: "fermi", sessionId: "s-2", me: "p1", finished: true, results: { players: [{ player: "p1", total: 42 }] } }, s);
    expect(loadGameResults("fruit", s)).toHaveLength(1);
    expect(loadGameResults("fermi", s)[0].score).toBe(42);
  });

  test("clear removes everything or just one game", () => {
    const s = fakeStore();
    recordFinishedResult({ ...base, finished: true, results: fruitResults }, s);
    recordFinishedResult({ slug: "fermi", sessionId: "s-2", me: "p1", finished: true, results: { players: [{ player: "p1", total: 42 }] } }, s);
    clearGameResults("fruit", s);
    expect(loadGameResults("fruit", s)).toHaveLength(0);
    expect(loadGameResults("fermi", s)).toHaveLength(1);
    clearGameResults(undefined, s);
    expect(loadGameResults(undefined, s)).toHaveLength(0);
  });

  test("missing storage is a no-op, not a crash", () => {
    // Persistence is best-effort: the entry is still computed, just not stored.
    const entry = recordFinishedResult({ ...base, finished: true, results: fruitResults }, null);
    expect(entry?.score).toBe(96);
    expect(loadGameResults(undefined, null)).toEqual([]);
  });
});

describe("summarizeResult per game shape", () => {
  test("math: top-level result object", () => {
    const s = summarizeResult("math", "p1", { score: 80, correct: 100, incorrect: 20, speed_points: 1 });
    expect(s.score).toBe(80);
    expect(s.detail).toContain("100 correct");
  });

  test("fermi/makeme/nextcard: my row from players array", () => {
    const results = { players: [
      { player: "p2", total: 99, rank: 1 },
      { player: "p1", total: 42, rank: 2 },
    ] };
    expect(summarizeResult("fermi", "p1", results).score).toBe(42);
    expect(summarizeResult("fermi", "p1", results).rank).toBe(2);
    const mm = summarizeResult("makeme", "p1", { players: [{ player: "p1", base_score: 6.5 }] });
    expect(mm.score).toBe(6.5);
    const nc = summarizeResult("nextcard", "p1", { players: [{ player: "p1", skill: 10, final_balance: 120 }] });
    expect(nc.label).toBe("skill");
  });

  test("unknown game falls back to a neutral completed label", () => {
    const s = summarizeResult("unknown", "p1", { players: [] });
    expect(s.score).toBeNull();
    expect(s.label).toBe("completed");
  });

  test("falls back to the first row when the player is missing", () => {
    const s = summarizeResult("cards", "ghost", { players: [{ player: "p1", profit: 5, cash: 5, position: 1 }] });
    expect(s.score).toBe(5);
  });
});
