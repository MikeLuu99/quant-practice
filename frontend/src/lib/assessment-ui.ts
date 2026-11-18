import { escapeHTML as esc } from "./logic";

export type AssessmentChoice = { id: string; text: string };
export type AssessmentQuestion = {
  question_id: string; prompt: string; family: string; response_type: string;
  choices?: AssessmentChoice[]; assets?: any; units?: string;
};
export function cubeSVG(c: {top:string;front:string;right:string}): string {
  return `<svg class="cube-view" viewBox="0 0 140 145" role="img" aria-label="Cube: top ${esc(c.top)}, front ${esc(c.front)}, right ${esc(c.right)}"><polygon points="70,8 126,38 70,69 14,38" class="cube-top"/><polygon points="14,38 70,69 70,133 14,101" class="cube-front"/><polygon points="70,69 126,38 126,101 70,133" class="cube-right"/><text x="70" y="45">${esc(c.top)}</text><text x="42" y="91">${esc(c.front)}</text><text x="98" y="91">${esc(c.right)}</text></svg>`;
}
export function renderAssessmentAsset(q: AssessmentQuestion): string {
  const a=q.assets;
  if (!a) return "";
  switch(a.kind) {
    case "sequence": return `<div class="sequence-tokens" aria-label="Sequence">${a.tokens.map((t:string)=>`<span>${esc(t)}</span>`).join("")}${a.task!=="anomaly" ? '<span class="sequence-unknown">?</span>' : ""}</div>`;
    case "arrows": return `<div class="sequence-tokens" aria-label="Arrow pattern">${a.orientations.map((n:number)=>`<span>${["↑","→","↓","←"][n]}</span>`).join("")}<span class="sequence-unknown">?</span></div>`;
    case "table": return `<div class="table-scroll"><table class="tbl assessment-data"><thead><tr>${a.headers.map((h:string)=>`<th>${esc(h)}</th>`).join("")}</tr></thead><tbody>${a.rows.map((r:string[])=>`<tr>${r.map(c=>`<td>${esc(c)}</td>`).join("")}</tr>`).join("")}</tbody></table></div>`;
    case "cube": return `<div class="cube-net" role="img" aria-label="Cube net: ${esc(a.net.map((f:any)=>`face ${f.label} at column ${f.x+1}, row ${f.y+1}`).join('; '))}">${a.net.map((f:any)=>`<span style="grid-column:${Number(f.x)+1};grid-row:${Number(f.y)+1}">${esc(f.label)}</span>`).join("")}</div><p class="field-note">The labels have no directional texture. Rotate the folded cube; never reflect it.</p>`;
    case "grid": return `<div class="grid-experiment"><div class="letter-board"><div class="letter-heading">ROW / LETTERS / P(ROW)</div>${a.rows.map((r:string[],i:number)=>`<div class="letter-row"><span>${i+1}</span><strong>${esc(r[0])}</strong><strong>${esc(r[1])}</strong><span>${esc(a.probabilities[i])}</span></div>`).join("")}</div><div><p class="grid-rules">${esc(a.rules)}</p><div class="grid-bets">${a.bets.map((b:any)=>`<div><span class="eyebrow">Bet ${esc(b.id.toUpperCase())}</span><b>${b.components.map((c:any)=>`${c.sign===1?'+':'−'}${esc(c.letter)}`).join(' ')}</b><span class="mono">cost ${esc(b.cost)}</span></div>`).join("")}</div></div></div>`;
    default: return "";
  }
}
export function renderAssessmentInputs(q: AssessmentQuestion, saved: any): string {
  if(q.response_type==="single_choice") return `<fieldset class="assessment-choices ${q.assets?.kind==='cube'?'cube-choices':''}"><legend class="sr-only">Choose one answer</legend>${(q.choices||[]).map((c,i)=>`<label class="assessment-choice"><input type="radio" name="choice" value="${esc(c.id)}" ${saved?.answer===c.id?'checked':''}/><span class="choice-key">${i+1}</span>${q.assets?.kind==='cube'?cubeSVG(q.assets.candidates.find((v:any)=>v.id===c.id)):''}<span>${esc(c.text)}</span></label>`).join("")}</fieldset>`;
  if(q.response_type==="interval") return `<div class="interval-fields"><label class="field-label">Lower estimate<input id="interval-low" inputmode="decimal" autocomplete="off" placeholder="Lower bound" value="${esc(saved?.low||'')}"/></label><span aria-hidden="true">—</span><label class="field-label">Upper estimate<input id="interval-high" inputmode="decimal" autocomplete="off" placeholder="Upper bound" value="${esc(saved?.high||'')}"/></label></div><p class="field-note">80% interval · positive bounds · fractions, decimals and scientific notation accepted.</p><label class="field-label" for="interval-reasoning">Explain your reasoning <span class="muted">(for your review)</span></label><textarea id="interval-reasoning" rows="4" maxlength="4000" placeholder="Break the estimate into assumptions…">${esc(saved?.reasoning||'')}</textarea>`;
  return `<label class="field-label" for="assessment-answer">${q.response_type==='token'?'Your next letter':'Your answer'}${q.units?` · ${esc(q.units)}`:''}</label><input id="assessment-answer" class="assessment-answer" inputmode="${q.response_type==='token'?'text':'decimal'}" autocomplete="off" spellcheck="false" placeholder="${q.response_type==='token'?'A–Z':'Number, decimal or fraction'}" value="${esc(saved?.answer||'')}"/><span class="field-note">${q.response_type==='token'?'Letters are case insensitive.':'Exact answers: 0.75 = 3/4 = 6/8. No thousands separators.'}</span>`;
}
export function responseSummary(response:any, question?:AssessmentQuestion):string {
  if (question?.response_type === "single_choice" && response?.answer) {
    const choice = question.choices?.find(c=>c.id===response.answer);
    if (choice) return `${choice.id.toUpperCase()}: ${choice.text}`;
  }
  if(response?.low!==undefined) return `[${response.low}, ${response.high}]`;
  return response?.answer || "Unanswered";
}
