import { decisionKey, fmtClock } from "./logic";
import { recordFinishedResult } from "./scores";

export type ActionOutput = {
  accepted: boolean; reason?: string; server_sequence?: number; execution?: any;
  public_changes?: any; private_changes?: any; round_reward?: number; game_over?: boolean;
};
export async function api<T = any>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, { ...init, headers: { "Content-Type": "application/json", ...init?.headers } });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || `Request failed (${res.status})`);
  return body as T;
}
export async function listGames(): Promise<any[]> { return (await api("/games")).games; }
export async function createSession(game: string, players: string[], config: any, seed?: number) {
  return api<{ session_id: string; players: string[]; snapshots: Record<string, any> }>("/sessions", {
    method: "POST", body: JSON.stringify({ game, players, config, seed }),
  });
}
export async function getState(sid: string, player: string) {
  return api<{ game: string; players: string[]; finished: boolean; state: any }>(`/sessions/${sid}/state?player=${encodeURIComponent(player)}`);
}
export async function sendAction(sid: string, body: Record<string, any>): Promise<ActionOutput> {
  return (await api<{ output: ActionOutput }>(`/sessions/${sid}/action`, { method: "POST", body: JSON.stringify(body) })).output;
}
export async function getResults(sid: string) { return api<{ finished: boolean; results: any }>(`/sessions/${sid}/results`); }
export async function getTape(sid: string): Promise<any> { return api(`/sessions/${sid}/tape`); }
let actionCounter = 0;
export function nextActionId(): string { return `a-${Date.now().toString(36)}-${++actionCounter}`; }
export interface Controller {
  sid: string; players: string[]; me: string; state: any | null;
  act: (action: string, payload?: any) => Promise<ActionOutput>;
  refresh: () => Promise<void>;
}
export interface MountOptions {
  slug: string; defaults: Record<string, any>;
  render: (c: Controller, board: HTMLElement) => void;
  renderResults: (c: Controller, results: any, box: HTMLElement) => void;
  pollMs?: number;
}

const reasons: Record<string, string> = {
  INVALID_VALUE: "Check your entry and try again.", STALE_MARKET: "The quote changed. Review the current prices and try again.",
  INSUFFICIENT_BALANCE: "Your stake exceeds your balance.", INSUFFICIENT_STAKES: "Your total stakes exceed your balance.",
  ALREADY_ACTED: "Your decision is already locked for this round.", NOT_YOUR_TURN: "It is another player's turn.",
  MARKET_CLOSED: "This market has closed.", RATE_LIMIT: "Wait a moment before your next trade.", NO_CAPACITY: "This quote has already been filled.",
};

export function mountBoard(opts: MountOptions): void {
  const setup = document.getElementById("setup")!;
  const board = document.getElementById("board")!;
  const results = document.getElementById("results")!;
  const errBox = document.getElementById("setup-error")!;
  const tools = document.getElementById("session-tools")!;
  const notice = document.getElementById("action-notice")!;
  const connection = document.getElementById("connection-status")!;
  const form = document.getElementById("setup-form") as HTMLFormElement;
  const url = new URL(location.href);
  let sid = url.searchParams.get("s") || "";
  let me = url.searchParams.get("me") || "p1";
  let timer: ReturnType<typeof setTimeout> | undefined;
  let finished = false;
  let pending = false;
  let generation = 0;
  let requestNumber = 0;
  let renderedKey = "";
  let renderedSignature = "";

  function notify(text: string, error = false) {
    notice.textContent = text;
    notice.className = `action-notice ${error ? "is-error" : ""}`;
    notice.hidden = !text;
  }
  const c: Controller = {
    sid, players: [], me, state: null,
    async act(action, payload) {
      if (pending) return { accepted: false, reason: "ACTION_PENDING" };
      if (finished) return { accepted: false, reason: "MARKET_CLOSED" };
      pending = true;
      board.setAttribute("aria-busy", "true");
      // Invalidate reads started before the action. Polling resumes with a
      // fresh snapshot; an old response must never overwrite the fill.
      requestNumber++;
      stopPolling();
      const ownGeneration = generation;
      const ownPlayer = me;
      try {
        const out = await sendAction(sid, { action_id: nextActionId(), player_id: ownPlayer,
          market_version: c.state?.market_version, action, payload: payload ?? {} });
        if (ownGeneration !== generation || ownPlayer !== me) return out;
        if (!out.accepted) notify(reasons[out.reason || ""] || `Action rejected: ${out.reason}`, true);
        else if (action === "ANSWER") notify(`${out.execution?.correct ? "Correct. +1 point" : "Incorrect. −1 point"} · Answer: ${out.execution?.correct_answer}`);
        else if (out.execution?.side || action === "BUY" || action === "SELL" || action.endsWith("ETF")) {
          const price = out.execution?.price;
          notify(`${action.replaceAll("_", " ")} filled${price != null ? ` at ${price}` : ""}${out.round_reward != null ? ` · ${fmtNum(out.round_reward)} profit` : ""}`);
        } else notify(action === "PASS" ? "Round passed." : action === "REVEAL_HISTORY" ? "Your book is now visible below the question." : "Decision locked.");
        return out;
      } catch (e) {
        notify(`Could not send action. ${(e as Error).message} Try again.`, true);
        return { accepted: false, reason: "CONNECTION_ERROR" };
      } finally {
        pending = false;
        board.removeAttribute("aria-busy");
        schedulePoll();
      }
    },
    async refresh() {
      const request = ++requestNumber;
      const ownGeneration = generation;
      const ownPlayer = me;
      const data = await getState(sid, me);
      if (request !== requestNumber || ownGeneration !== generation || ownPlayer !== me) return;
      if (data.game && data.game !== opts.slug) throw new Error("This session belongs to another game. Start a new session here.");
      const newKey = decisionKey(me, data.state);
      const signature = JSON.stringify([me, { ...data.state, time_remaining_ms: 0 }]);
      const keepDraft = newKey === renderedKey;
      if (signature !== renderedSignature) {
        const inputs = Array.from(board.querySelectorAll<HTMLInputElement>("input"));
        const active = document.activeElement as HTMLInputElement | null;
        const drafts = inputs.map((el, i) => ({ key: el.id || el.dataset.event || String(i), value: el.value,
          active: active === el, start: el.selectionStart, end: el.selectionEnd }));
        c.state = data.state;
        finished = data.finished;
        c.players = data.players || c.players;
        const previousFocus = document.activeElement;
        opts.render(c, board);
        // Renderers may focus an answer field. Polling should not steal focus
        // from player controls or a button, nor erase a partly entered answer.
        if (keepDraft) {
          board.querySelectorAll<HTMLInputElement>("input").forEach((el, i) => {
            const saved = drafts.find((d) => d.key === (el.id || el.dataset.event || String(i)));
            if (!saved) return;
            el.value = saved.value;
            if (saved.active) {
              el.focus({ preventScroll: true });
              if (saved.start != null) el.setSelectionRange(saved.start, saved.end);
            }
          });
          if (previousFocus instanceof HTMLElement && previousFocus.isConnected && previousFocus !== document.body) previousFocus.focus({ preventScroll: true });
        } else if (!renderedKey || (previousFocus instanceof HTMLElement && (previousFocus === document.body || !previousFocus.isConnected))) {
          board.querySelector<HTMLInputElement>("input:not(:disabled)")?.focus({ preventScroll: true });
        }
        renderedKey = newKey;
        renderedSignature = signature;
      }
      c.state = data.state;
      finished = data.finished;
      c.players = data.players || c.players;
      board.querySelectorAll("[data-clock]").forEach((el) => el.textContent = fmtClock(finished ? 0 : data.state.time_remaining_ms ?? 0));
      board.querySelectorAll<HTMLInputElement>("input").forEach((el) => {
        if (!el.labels?.length && !el.hasAttribute("aria-label")) el.setAttribute("aria-label", el.placeholder || "Game input");
      });
      if (finished) board.querySelectorAll<HTMLButtonElement>("button").forEach((b) => b.disabled = true);
      for (const id of ["show-results", "show-tape"]) {
        const button = document.getElementById(id) as HTMLButtonElement;
        button.disabled = !finished;
        button.title = finished ? "" : "Available when the session ends";
      }
      connection.textContent = finished ? "Session complete" : "Connected · live";
      connection.classList.remove("offline");
      if (finished) {
        stopPolling();
        const res = await getResults(sid);
        if (request === requestNumber && ownGeneration === generation) showResultsBox(res.results);
        saveScore(res.results);
      }
    },
  };
  // Past scores persist per completed session; unfinished sessions are never
  // recorded, and one session never produces two entries.
  const savedSessions = new Set<string>();
  function saveScore(res: any) {
    if (savedSessions.has(sid)) return;
    const entry = recordFinishedResult({ slug: opts.slug, sessionId: sid, me, finished, results: res });
    if (entry) {
      savedSessions.add(sid);
      window.dispatchEvent(new CustomEvent("quant:scores-updated"));
    }
  }
  function showResultsBox(res: any) {
    results.hidden = false;
    results.innerHTML = "<h2>Session results</h2>";
    opts.renderResults(c, res, results);
  }
  function stopPolling() { if (timer) clearTimeout(timer); timer = undefined; }
  function schedulePoll() {
    stopPolling();
    if (!sid || finished) return;
    timer = setTimeout(async () => {
      if (pending) { schedulePoll(); return; }
      try { await c.refresh(); }
      catch { connection.textContent = "Connection interrupted · retrying"; connection.classList.add("offline"); }
      schedulePoll();
    }, opts.pollMs ?? 900);
  }
  function wirePlayerSwitch() {
    const wrap = document.getElementById("player-switch")!;
    wrap.replaceChildren();
    if (c.players.length < 2) return;
    const label = document.createElement("label");
    label.textContent = "Playing as ";
    const sel = document.createElement("select");
    sel.id = "me-select";
    sel.setAttribute("aria-label", "Playing as");
    for (const p of c.players) {
      const o = document.createElement("option");
      o.value = p; o.textContent = p; o.selected = p === me; sel.appendChild(o);
    }
    sel.addEventListener("change", async () => {
      me = sel.value; c.me = me; generation++; c.state = null; renderedKey = ""; renderedSignature = "";
      board.replaceChildren(); results.hidden = true; notify("");
      updateURL();
      try { await c.refresh(); } catch (e) { notify((e as Error).message, true); }
      schedulePoll();
    });
    label.appendChild(sel); wrap.appendChild(label);
  }
  function updateURL() {
    url.searchParams.set("s", sid); url.searchParams.set("me", me);
    history.replaceState(null, "", url.toString());
  }
  function showBoard() {
    setup.hidden = true; board.hidden = false; tools.hidden = false;
    document.body.classList.add("is-playing");
    c.sid = sid; c.me = me;
    updateURL(); wirePlayerSwitch();
  }
  async function startSession(players: string[], seed?: number) {
    stopPolling(); generation++; finished = false; renderedKey = ""; renderedSignature = "";
    const data = await createSession(opts.slug, players, opts.defaults, seed);
    sid = data.session_id; c.players = data.players;
    if (!c.players.includes(me)) me = c.players[0];
    c.state = null; board.replaceChildren(); results.hidden = true; notify("");
    document.getElementById("tape-box")!.hidden = true;
    showBoard(); await c.refresh(); schedulePoll();
  }
  if (sid) {
    getState(sid, me).then(async (data) => {
      if (data.game && data.game !== opts.slug) throw new Error("This session belongs to another game.");
      c.players = data.players; showBoard(); await c.refresh(); schedulePoll();
    }).catch((e) => {
      errBox.textContent = `Could not restore session: ${e.message} Start a new session below.`;
      setup.hidden = false; board.hidden = true; tools.hidden = true;
      document.body.classList.remove("is-playing");
    });
  }
  form.addEventListener("submit", async (ev) => {
    ev.preventDefault(); errBox.textContent = "";
    const fd = new FormData(form);
    const players = String(fd.get("players") || "p1").split(",").map((p) => p.trim()).filter(Boolean);
    const rawSeed = String(fd.get("seed") || "").trim();
    const seed = rawSeed ? Number(rawSeed) : undefined;
    if (!players.length || new Set(players).size !== players.length) { errBox.textContent = "Enter a unique name for each player."; return; }
    if (seed !== undefined && (!Number.isSafeInteger(seed) || seed < 1)) { errBox.textContent = "Use a positive whole number for the seed."; return; }
    const btn = form.querySelector("button[type=submit]") as HTMLButtonElement;
    btn.disabled = true; btn.textContent = "Starting…";
    try { await startSession(players, seed); } catch (e) { errBox.textContent = (e as Error).message; setup.hidden = false; }
    finally { btn.disabled = false; btn.textContent = "Start playing ↗"; }
  });
  document.getElementById("new-session")?.addEventListener("click", () => {
    stopPolling(); generation++; requestNumber++; sid = ""; c.sid = ""; c.state = null;
    setup.hidden = false; board.hidden = true; results.hidden = true; tools.hidden = true;
    document.getElementById("tape-box")!.hidden = true; notify("");
    document.body.classList.remove("is-playing");
    url.searchParams.delete("s"); url.searchParams.delete("me"); history.replaceState(null, "", url.toString());
  });
  document.getElementById("show-results")?.addEventListener("click", async () => {
    try {
      const res = (await getResults(sid)).results;
      showResultsBox(res);
      saveScore(res);
    }
    catch (e) { notify((e as Error).message, true); }
  });
  document.getElementById("show-tape")?.addEventListener("click", async () => {
    const pre = document.getElementById("tape-box")!;
    pre.hidden = !pre.hidden;
    if (!pre.hidden) {
      try { pre.textContent = JSON.stringify(await getTape(sid), null, 2); }
      catch (e) { notify((e as Error).message, true); }
    }
  });
  window.addEventListener("pagehide", () => { generation++; stopPolling(); });
}

export function renderPlayerTable(box: HTMLElement, players: any[], keys: [string, string][]): void {
  const wrap = document.createElement("div"); wrap.className = "table-scroll";
  const tbl = document.createElement("table"); tbl.className = "tbl";
  const thead = tbl.createTHead(); const head = thead.insertRow();
  for (const [, label] of keys) { const th = document.createElement("th"); th.textContent = label; head.appendChild(th); }
  const tbody = tbl.createTBody();
  for (const p of [...players].sort((a, b) => (a.rank ?? 0) - (b.rank ?? 0))) {
    const tr = tbody.insertRow();
    for (const [key] of keys) {
      const td = tr.insertCell(); const v = p[key];
      td.className = typeof v === "number" ? "num" : "";
      td.textContent = typeof v === "number" ? fmtNum(v) : v == null ? "—" : String(v);
    }
  }
  wrap.appendChild(tbl); box.appendChild(wrap);
}
export function fmtNum(n: number, d = 2): string { return Number(n).toLocaleString("en-US", { maximumFractionDigits: d }); }
