// A single card renderer for tables, previews, and the reveal history.
import { escapeHTML } from "./logic";

type Card = { label?: string; rank?: number } | null;
const suits: Record<string, [string, string]> = {
  S: ["♠", "spades"], H: ["♥", "hearts"], D: ["♦", "diamonds"], C: ["♣", "clubs"],
};
const pipLayouts: Record<number, [number, number][]> = {
  1: [[1, 2]], 2: [[1, 0], [1, 4]], 3: [[1, 0], [1, 2], [1, 4]],
  4: [[0, 0], [2, 0], [0, 4], [2, 4]],
  5: [[0, 0], [2, 0], [1, 2], [0, 4], [2, 4]],
  6: [[0, 0], [2, 0], [0, 2], [2, 2], [0, 4], [2, 4]],
  7: [[0, 0], [2, 0], [1, 1], [0, 2], [2, 2], [0, 4], [2, 4]],
  8: [[0, 0], [2, 0], [1, 1], [0, 2], [2, 2], [1, 3], [0, 4], [2, 4]],
  9: [[0, 0], [2, 0], [0, 1], [2, 1], [1, 2], [0, 3], [2, 3], [0, 4], [2, 4]],
  10: [[0, 0], [2, 0], [1, 1], [0, 1.35], [2, 1.35], [0, 2.65], [2, 2.65], [1, 3], [0, 4], [2, 4]],
};

export function playingCard(card: Card = null, size: "normal" | "small" = "normal"): string {
  const cls = `playing-card${size === "small" ? " playing-card--small" : ""}`;
  if (!card?.label) return `<span class="${cls} card-back" role="img" aria-label="Face-down card"><span class="card-back-pattern" aria-hidden="true"><span>QG</span></span></span>`;
  const label = card.label;
  const rank = label.slice(0, -1);
  const suit = suits[label.slice(-1)];
  if (!suit || !/^(A|[2-9]|10|J|Q|K)$/.test(rank)) return "";
  const n = rank === "A" ? 1 : Number(rank);
  const [symbol, name] = suit;
  const corner = `<span>${rank}</span><span>${symbol}</span>`;
  const picture = Number.isNaN(n)
    ? `<span class="card-court"><span>${symbol}</span><b>${rank}</b><span>${symbol}</span></span>`
    : `<span class="card-pips" aria-hidden="true">${pipLayouts[n].map(([x, y]) => `<span class="pip${y > 2 ? " pip-inverted" : ""}" style="left:${x * 40 + 10}%;top:${y * 22 + 6}%">${symbol}</span>`).join("")}</span>`;
  return `<span class="${cls} ${name === "hearts" || name === "diamonds" ? "card-red" : "card-black"}" role="img" aria-label="${escapeHTML(rank)} of ${name}"><span class="card-corner" aria-hidden="true">${corner}</span>${picture}<span class="card-corner card-corner-bottom" aria-hidden="true">${corner}</span></span>`;
}
