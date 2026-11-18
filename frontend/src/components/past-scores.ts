import { loadGameResults, clearGameResults, type ScoreEntry } from "../lib/scores";

const slug = (document.getElementById("past-scores") as HTMLElement | null)?.dataset.slug || "";
const box = document.getElementById("past-scores") as HTMLElement | null;

function esc(s: string): string {
  return s.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
}

function when(at: string): string {
  const d = new Date(at);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleDateString(undefined, { month: "short", day: "numeric" }) + " " + d.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
}

function render(): void {
  if (!box) return;
  const rows = loadGameResults(slug || undefined);
  if (!rows.length) {
    box.hidden = true;
    box.replaceChildren();
    return;
  }
  const heading = document.createElement("h2");
  heading.textContent = "Past scores";
  const clear = document.createElement("button");
  clear.className = "btn sm past-scores-clear";
  clear.type = "button";
  clear.textContent = "Clear";
  clear.addEventListener("click", () => {
    clearGameResults(slug || undefined);
    window.dispatchEvent(new CustomEvent("quant:scores-updated"));
  });
  heading.appendChild(clear);
  const list = document.createElement("ol");
  list.className = "past-scores-list";
  for (const r of rows.slice(0, 10)) list.appendChild(row(r));
  box.replaceChildren(heading, list);
  box.hidden = false;
}

function row(r: ScoreEntry): HTMLLIElement {
  const li = document.createElement("li");
  const scoreText = r.score == null ? esc(r.label) : `${esc(r.label)}: <b>${esc(String(r.score))}</b>`;
  li.innerHTML = `<span class="past-scores-when">${esc(when(r.at))}</span>
    <span class="past-scores-score">${scoreText}${r.rank != null ? ` · rank ${esc(String(r.rank))}` : ""}</span>
    <span class="past-scores-detail">${esc(r.detail || "")}${r.player ? ` · ${esc(r.player)}` : ""}</span>`;
  return li;
}

window.addEventListener("quant:scores-updated", render);
render();
