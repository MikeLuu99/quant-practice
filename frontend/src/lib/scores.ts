// Past-score persistence. Only completed sessions are recorded; an entry is
// keyed by session id so retries and result reloads never duplicate rows.

export interface ScoreEntry {
  at: string; // ISO timestamp
  game: string; // engine slug (or "trainer"/"assessment")
  session: string;
  player: string;
  score: number | null;
  label: string; // human label for the score metric
  detail?: string;
  rank?: number;
}

const KEY = "quant-practice-game-results";
const CAP = 100;

type MinimalStorage = { getItem(k: string): string | null; setItem(k: string, v: string): void; removeItem(k: string): void };

function storage(): MinimalStorage | null {
  try {
    if (typeof localStorage !== "undefined") return localStorage;
  } catch {}
  return null;
}

function readAll(store: MinimalStorage | null): ScoreEntry[] {
  if (!store) return [];
  try {
    const raw = JSON.parse(store.getItem(KEY) || "[]");
    return Array.isArray(raw) ? raw.filter((e) => e && typeof e === "object" && typeof e.session === "string") : [];
  } catch {
    return [];
  }
}

function writeAll(store: MinimalStorage | null, entries: ScoreEntry[]): void {
  if (!store) return;
  try {
    store.setItem(KEY, JSON.stringify(entries.slice(0, CAP)));
  } catch {}
}

// Which metric is "the score" for each game, plus a short detail string.
// Works for both per-player rows and top-level result shapes (math).
const METRICS: Record<string, { label: string; primary: (r: any) => number | null | undefined; detail?: (r: any) => string | undefined }> = {
  fruit: { label: "final score", primary: (r) => r.final_score, detail: (r) => `${fmt(r.raw_profit)} raw · ${pct(r.accuracy)} first-click accuracy` },
  math: { label: "net points", primary: (r) => r.score, detail: (r) => `${fmt(r.correct)} correct · ${fmt(r.incorrect)} wrong${r.speed_points != null ? ` · +${r.speed_points} speed` : ""}` },
  cards: { label: "profit", primary: (r) => r.profit, detail: (r) => `cash ${fmt(r.cash)} · position ${fmt(r.position)}` },
  marble: { label: "profit", primary: (r) => r.profit, detail: (r) => `${fmt(r.edge_sum)} edge points · rank score ${fmt(r.rank_score)}` },
  etf: { label: "total profit", primary: (r) => r.total_profit, detail: (r) => `${fmt(r.profitable_trades)}/${fmt(r.trades)} profitable` },
  nextcard: { label: "skill", primary: (r) => r.skill, detail: (r) => `balance ${fmt(r.final_balance)} · decisions ${pct(r.decision_fraction)}` },
  prob: { label: "skill", primary: (r) => r.skill, detail: (r) => `balance ${fmt(r.final_balance)} · decisions ${pct(r.decision_fraction)}` },
  fermi: { label: "total", primary: (r) => r.total, detail: (r) => `rank ${fmt(r.rank)}` },
  makeme: { label: "base score", primary: (r) => r.base_score, detail: (r) => `${fmt(r.book_points)} book · ${fmt(r.estimation_bonus)} estimate` },
};

function fmt(n: any): string {
  return typeof n === "number" ? Number(n).toLocaleString("en-US", { maximumFractionDigits: 2 }) : "—";
}
function pct(x: any): string {
  return typeof x === "number" ? `${Math.round(x * 100)}%` : "—";
}

// myRow picks the acting player's row from a players-shaped result, or the
// whole result object when the game reports top-level fields (math).
export function summarizeResult(slug: string, me: string, results: any): { score: number | null; label: string; detail?: string; rank?: number } {
  const metric = METRICS[slug];
  if (!metric) return { score: null, label: "completed" };
  const players = Array.isArray(results?.players) ? results.players : null;
  const row = players ? players.find((p: any) => p.player === me) ?? players[0] : results;
  const primary = metric.primary(row ?? {});
  return {
    score: typeof primary === "number" ? primary : null,
    label: metric.label,
    detail: metric.detail?.(row ?? {}),
    rank: typeof row?.rank === "number" ? row.rank : undefined,
  };
}

export function recordFinishedResult(
  input: { slug: string; sessionId: string; me: string; finished: boolean; results: any },
  store: MinimalStorage | null = storage(),
): ScoreEntry | null {
  // Unfinished challenges/tasks are never recorded — by contract.
  if (!input.finished || !input.sessionId || !input.results) return null;
  const existing = readAll(store);
  if (existing.some((e) => e.session === input.sessionId)) return null; // idempotent per session
  const summary = summarizeResult(input.slug, input.me, input.results);
  const entry: ScoreEntry = {
    at: new Date().toISOString(),
    game: input.slug,
    session: input.sessionId,
    player: input.me,
    score: summary.score,
    label: summary.label,
    detail: summary.detail,
    rank: summary.rank,
  };
  writeAll(store, [entry, ...existing]);
  return entry;
}

export function loadGameResults(slug?: string, store: MinimalStorage | null = storage()): ScoreEntry[] {
  const all = readAll(store);
  const filtered = slug ? all.filter((e) => e.game === slug) : all;
  return filtered.sort((a, b) => b.at.localeCompare(a.at));
}

export function clearGameResults(slug?: string, store: MinimalStorage | null = storage()): void {
  if (!slug) {
    store?.removeItem(KEY);
    return;
  }
  writeAll(store, readAll(store).filter((e) => e.game !== slug));
}
