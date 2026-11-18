// Pure helpers used by game pages and unit-tested with bun test.

export function fmtClock(ms: number): string {
  if (ms < 0 || Number.isNaN(ms)) ms = 0;
  const s = Math.floor(ms / 1000);
  const m = Math.floor(s / 60);
  return m > 0 ? `${m}:${String(s % 60).padStart(2, "0")}` : `${s}s`;
}

export function parseStake(raw: string, balance: number): number {
  const v = Number(raw);
  if (!Number.isFinite(v) || v <= 0) return NaN;
  const rounded = Math.round(v * 100) / 100;
  return rounded > 0 && rounded <= balance ? rounded : NaN;
}

export function isCardRed(label: string): boolean {
  return label.endsWith("H") || label.endsWith("D");
}

export function outcomeClass(outcome: string): string {
  switch (outcome) {
    case "WIN":
      return "good";
    case "LOSS":
      return "bad";
    case "PUSH":
      return "warn";
    default:
      return "";
  }
}

// Fruit Market: value = multiplier x total apples x total oranges.
export function fruitValue(bags: { apples: number; oranges: number }[], mult: number): number {
  const a = bags.reduce((s, b) => s + b.apples, 0);
  const o = bags.reduce((s, b) => s + b.oranges, 0);
  return Math.round(mult * a * o);
}

// Kelly fraction for even-money push-allowed bets: (pW - pL) / (pW + pL).
export function kellyEvenMoney(pW: number, pL: number): number {
  if (pW + pL <= 0) return 0;
  return Math.max(0, (pW - pL) / (pW + pL));
}

// Kelly fraction for decimal odds D (stake returned included).
export function kellyDecimal(p: number, D: number): number {
  const b = D - 1;
  if (b <= 0) return 0;
  const ev = p * b - (1 - p);
  return Math.max(0, ev / b);
}

export function multiplicativeError(x: number, y: number): number {
  return Math.max(x / y, y / x);
}

export function fermiPoints(r: number): number {
  if (r <= 1.1) return 10;
  if (r <= 1.25) return 8;
  if (r <= 1.5) return 6;
  if (r <= 2) return 4;
  if (r <= 3) return 2;
  return 0;
}

export function escapeHTML(value: unknown): string {
  return String(value ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]!));
}

// Drafts belong to a specific player and decision. Never carry a previous
// answer into the next question, round, maker turn, or player's private view.
export function decisionKey(player: string, s: any): string {
  return JSON.stringify([player, s.phase, s.round, s.index, s.turn, s.rotation,
    s.turn_in_rotation, s.active_maker, s.question?.question_id, s.book_question?.index]);
}
