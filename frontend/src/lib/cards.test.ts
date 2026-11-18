import { describe, expect, test } from "bun:test";
import { playingCard } from "./cards";
import { decisionKey, escapeHTML, parseStake } from "./logic";

describe("playing cards", () => {
  test("renders every card in a standard deck with its rank and suit", () => {
    for (const suit of ["S", "H", "D", "C"]) {
      for (const rank of ["A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"]) {
        const html = playingCard({ label: rank + suit });
        expect(html).toContain('role="img"');
        expect(html).toContain(`<span>${rank}</span>`);
        expect(html).not.toContain("undefined");
        expect(html.includes("card-red")).toBe(suit === "H" || suit === "D");
      }
    }
  });
  test("conceals unknown cards and refuses malformed labels", () => {
    expect(playingCard()).toContain('aria-label="Face-down card"');
    expect(playingCard()).not.toContain("card-corner");
    expect(playingCard({label: '<script>S'})).toBe("");
    expect(playingCard({label: '14H'})).toBe("");
  });
  test("number cards have the correct number of pips", () => {
    for (let rank = 2; rank <= 10; rank++) {
      const html = playingCard({label: `${rank}S`});
      expect((html.match(/class="pip[" ]/g) || []).length).toBe(rank);
    }
  });
});

describe("live board decisions", () => {
  const question = { phase: "QUESTION", round: 1, question: { question_id: "q1" }, time_remaining_ms: 10_000 };
  test("polling and price changes do not invalidate a draft", () => {
    expect(decisionKey("alice", question)).toBe(decisionKey("alice", {...question, time_remaining_ms: 9_000}));
    expect(decisionKey("alice", {phase: "QUOTE", turn: 2, market_version: 3})).toBe(decisionKey("alice", {phase: "QUOTE", turn: 2, market_version: 4}));
  });
  test("switching players, rounds, questions, or phases clears a draft", () => {
    const key = decisionKey("alice", question);
    expect(key).not.toBe(decisionKey("bob", question));
    expect(key).not.toBe(decisionKey("alice", {...question, round: 2}));
    expect(key).not.toBe(decisionKey("alice", {...question, phase: "REVEAL"}));
    expect(key).not.toBe(decisionKey("alice", {...question, question: {question_id: "q2"}}));
    expect(decisionKey("alice", {phase: "BOOK", book_question: {index: 0}})).not.toBe(decisionKey("alice", {phase: "BOOK", book_question: {index: 1}}));
  });
  test("escapes player names and reviewed answers before HTML rendering", () => {
    expect(escapeHTML('<img src=x onerror="alert(1)">')).toBe('&lt;img src=x onerror=&quot;alert(1)&quot;&gt;');
    expect(escapeHTML("Alice & Bob")).toBe("Alice &amp; Bob");
  });
  test("rejects stakes above balance or rounded down to zero", () => {
    for (const raw of ["101", "0.001", "Infinity", "-1"]) expect(Number.isNaN(parseStake(raw, 100))).toBe(true);
    expect(parseStake("100", 100)).toBe(100);
  });
});
