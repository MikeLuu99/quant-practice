import {escapeHTML as esc} from './logic';
import {renderAssessmentAsset,renderAssessmentInputs,cubeSVG} from './assessment-ui';
export const pretty=(s:string)=>s.replaceAll('_',' ').replaceAll('-',' ');
const button=(text:string,action:string,data='')=>`<button class="btn" type="button" data-action="${action}" ${data}>${text}</button>`;
const numberInput=(id:string,label:string,value:string)=>`<label class="field-label">${label}<input id="${id}" value="${value}" inputmode="decimal" required/></label>`;
function feedback(s:any):string{
 const f=s.feedback;if(!f)return '';
 const result=f.status?`<span class="badge">${esc(pretty(f.status))}</span>`:'';
 return `<div class="trainer-feedback" role="status">${result}${f.explanation?`<p>${esc(f.explanation)}</p>`:''}${f.answer!=null?`<p>Answer: <b class="mono">${esc(f.answer)}</b></p>`:''}${f.factor_error?`<p>Factor error: ${Number(f.factor_error).toPrecision(4)}×</p>`:''}${f.matching_attributes!=null?`<p><b>${f.matching_attributes}</b> matching attributes · ${f.remaining_candidates} candidates remain</p>`:''}${f.arrival_error_ms!=null?`<p>Arrival error: ${Math.round(f.arrival_error_ms)} ms</p>`:''}${f.bank_delta_cents!=null?`<p>Bank change: ${(f.bank_delta_cents/100).toFixed(2)} · ${esc(f.threshold_observation||`Popped at ${f.threshold}`)}</p>`:''}${f.excess_moves!=null?`<p>${f.state.moves} moves · ${f.minimum_moves} optimum · ${f.excess_moves} excess</p>`:''}${f.guesses_to_solve?`<p>Solved in ${f.guesses_to_solve} guesses.</p>`:''}${f.solution?.digits?`<p>Shown: <b class="mono">${esc(f.solution.digits)}</b> → ${esc(f.solution.answer)}</p>`:''}${f.solution?.targets?`<p>Blue cells: ${f.solution.targets.map((v:number)=>v+1).join(', ')}</p>`:''}${f.expression?`<p class="mono">${esc(f.expression)}</p>`:''}${f.fill?`<p>${esc(f.fill)}</p>`:''}</div>`;
}
export function cubeFace(label:string,style:string):string{
 const n=/^[1-6]$/.test(label)?Number(label)-1:label.charCodeAt(0)-65;const colors=['#e16f4f','#efba47','#6da4d2','#77a784','#c08ea8','#b1a37e'];
 const symbols:Record<string,string[]>={shapes:['●','■','◆','✚','✦','✿'],dice:['⚀','⚁','⚂','⚃','⚄','⚅'],numbers:['1','2','3','4','5','6'],letters:['A','B','C','D','E','F'],icons:['✦','✚','✿','◉','✥','❖'],patterns:['⊞','⊠','▦','❖','◉','✺']};
 return `<span class="trainer-face" style="${style==='colors'?`background:${colors[n%6]};`:''}"><b>${style==='colors'?'':symbols[style]?.[n%6]||label}</b><small>${esc(label)}</small></span>`;
}
function cubeAsset(q:any):string{
 const a=q.assets,style=a.face_style||'letters';return `<div class="cube-net trainer-cube-net" role="img" aria-label="Labelled ${esc(style)} cube net">${a.net.map((f:any)=>`<span style="grid-column:${f.x+1};grid-row:${f.y+1}">${cubeFace(f.label,style)}</span>`).join('')}</div><p class="field-note">Face labels identify the textures. Rotate the cube; never reflect it.</p><div class="trainer-cube-options">${a.candidates.map((c:any,i:number)=>`<button type="button" class="trainer-cube-option" data-answer="${esc(c.id)}" aria-label="Option ${i+1}: top ${esc(c.top)}, front ${esc(c.front)}, right ${esc(c.right)}"><b class="choice-key">${i+1}</b>${cubeSVG(c)}<span class="trainer-face-key">${[c.top,c.front,c.right].map(l=>cubeFace(l,style)).join('')}</span></button>`).join('')}</div>`;
}
function renderLesson(s:any):string{
 const l=s.scene?.lesson;if(!l||s.config.mode!=='learn')return '';
 return `<section class="trainer-lesson"><span class="eyebrow">Lesson ${s.scene.lesson_index} / 10</span><h3>${esc(l.title)}</h3><p>${esc(l.body)}</p><div class="trainer-formula">${esc(l.formula)}</div><p>${esc(l.example)}</p><details><summary>Explore the model</summary><p class="field-note">This calculator is separate from your scored quick check. Change the inputs to see the model respond.</p><div id="trainer-simulator" data-domain="${s.config.trainer_id}">${simulatorInputs(s.config.trainer_id)}</div><output id="trainer-simulator-output" class="trainer-formula"></output></details></section>`;
}
function simulatorInputs(domain:string):string{
 const fields:Record<string,[string,string,string][]>= {
 etf:[['a','A shares','2'],['p','A price','40'],['b','B shares','1'],['q','B price','30'],['d','Divisor','1']],
 options:[['s','Spot','100'],['v','Volatility (decimal)','.2'],['t','Maturity (years)','.25']],
 futures:[['s','Spot','100'],['c','Simple net carry (decimal)','.04'],['t','Maturity (years)','.5']],
 prediction:[['p','Prior probability','.4'],['l','Evidence likelihood ratio','3']]};
 return `<div class="trainer-fields">${fields[domain].map(([id,label,value])=>numberInput('sim-'+id,label,value)).join('')}</div>`;
}
export function simulatorValue(domain:string,v:Record<string,number>):string{
 let n=NaN,label='';if(domain==='etf'&&v.d>0){n=(v.a*v.p+v.b*v.q)/v.d;label='Basket NAV'}
 if(domain==='options'&&v.s>=0&&v.v>=0&&v.t>=0){n=.8*v.s*v.v*Math.sqrt(v.t);label='Approximate ATM straddle'}
 if(domain==='futures'&&v.t>=0){n=v.s*(1+v.c*v.t);label='Simple-carry fair value'}
 if(domain==='prediction'&&v.p>=0&&v.p<=1&&v.l>0){n=v.p*v.l/(1-v.p+v.p*v.l);label='Posterior probability'}
 return Number.isFinite(n)?`${label}: ${Number(n.toPrecision(8))}`:'Use finite values within the stated model.';
}
export function renderTrainerBoard(s:any):string{
 const phase=s.phase,scene=s.scene||{},kind=s.kind,move=s.move_state||{};
 if(phase==='READY')return `<div class="trainer-stimulus"><span class="eyebrow">Prepare</span><p>Get ready…</p></div>`;
 if(phase==='DONE')return `<div class="trainer-stimulus"><span class="eyebrow">Session complete</span><p>Repetition, reflected.</p></div>${feedback(s)}`;
 if(phase==='FEEDBACK')return `${feedback(s)}${button('Next repetition →','NEXT')}`;
 if(phase==='MASK')return `<div class="trainer-stimulus" aria-label="Memory mask"><b>+ + +</b><p>Hold it in mind.</p></div>`;
 let html=renderLesson(s);
 if(scene.case_screen)html+=`<div class="trainer-case-banner"><span class="eyebrow">Case screen ${scene.case_screen} / 9</span><h3>${esc(scene.screen_title)}</h3><p>${esc(scene.context)}</p></div>`;
 if(s.question){
  const q=s.question;html+=`<h3 class="trainer-question">${esc(q.prompt)}</h3>`;
  if(kind==='cube')return html+cubeAsset(q);
  html+=renderAssessmentAsset(q);
  if(kind==='fermi')html+=`<p class="field-note">Frozen original reference · ${esc(q.units||'')}</p>${scene.hint?`<p class="trainer-hint">${esc(scene.hint)}</p>`:''}<div class="row">${button('Reveal a hint','HINT')}</div><label class="field-label">Your decomposition<textarea id="trainer-reasoning" maxlength="4000" rows="4" placeholder="Assumptions × quantities × units…">${esc(move.reasoning||'')}</textarea></label>`;
  html+=`<form id="trainer-answer-form">${q.response_type==='digits'?`<label class="field-label">Rounded decimal</label><div class="trainer-digit-answer"><span>0.</span><input id="assessment-answer" inputmode="numeric" autocomplete="off" maxlength="${scene.places}" pattern="[0-9]{${scene.places}}" aria-label="${scene.places} decimal digits" required/></div>`:renderAssessmentInputs(q,null)}<div class="trainer-response-actions"><button class="btn primary" type="submit">${scene.lesson?'Check understanding':'Submit answer'} ↗</button>${['math','sequence','probability'].includes(kind)?button('Skip →','SKIP'):''}</div></form>`;return html;
 }
 switch(kind){
 case 'task-switch':case 'response-conflict':
  html+=`<div class="trainer-rule">${esc(scene.cue)}</div><div class="trainer-stimulus ${scene.location?'trainer-simon '+scene.location:''}">${scene.shape?`<span class="trainer-shape ${scene.shape}"></span>`:`<b>${esc(scene.display)}</b>`}</div><p class="field-note">${esc(scene.mapping)}</p><div class="trainer-response-pair">${kind==='task-switch'?button('← No','RESPOND','data-yes="false"')+button('Yes →','RESPOND','data-yes="true"'):button('← Left','RESPOND','data-side="LEFT"')+button('Right →','RESPOND','data-side="RIGHT"')}</div>`;break;
 case 'pincode':
  if(phase==='ENCODE')html+=`<div class="trainer-stimulus"><span class="eyebrow">Memorize · ${esc(scene.mode)}</span><b class="trainer-digit">${esc(scene.digit??'·')}</b></div>`;
  else html+=`<h3 class="trainer-question">${esc(pretty(scene.mode))} the ${scene.length} digits.</h3><form id="trainer-answer-form"><label class="field-label">Your recall<input id="assessment-answer" class="trainer-pin" inputmode="numeric" autocomplete="off" spellcheck="false" pattern="[0-9]{${scene.length}}" maxlength="${scene.length}" required aria-label="Recall ${scene.length} digits"/></label><button class="btn primary" type="submit">Lock recall ↗</button></form>`;break;
 case 'code-match':html+=`<span class="eyebrow">Find the exact match</span><div class="trainer-code-reference">${esc(scene.reference)}</div><div class="trainer-code-options">${scene.options.map((v:string,i:number)=>button(`<span class="choice-key">${i+1}</span><b>${esc(v)}</b>`,'RESPOND',`data-choice="${i}"`)).join('')}</div>`;break;
 case 'grid-recall':html+=`<h3 class="trainer-question">${phase==='ENCODE'?'Remember blue. Ignore red.':'Rebuild the blue pattern.'}</h3><div class="trainer-memory-grid">${Array.from({length:16},(_,i)=>`<button type="button" class="memory-cell ${(scene.blue||[]).includes(i)?'blue':''} ${(scene.red||[]).includes(i)?'red':''}" data-cell="${i}" aria-label="Cell ${i+1}" aria-pressed="false" ${phase!=='RESPONSE'?'disabled':''}><span>${phase==='RESPONSE'?i+1:''}</span>${(scene.red||[]).includes(i)?'<b>×</b>':''}</button>`).join('')}</div>${phase==='RESPONSE'?button('Submit pattern ↗','RECALL'):''}`;break;
 case 'intercept':html+=`<p class="field-note">One shot. Lead the falling ball to a crossing. ${scene.occluded?'Its final segment will disappear.':''}</p><canvas id="trainer-motion" width="800" height="520" aria-label="Falling ball with four cannon trajectories"></canvas><div class="trainer-cannon-controls">${['Q','A','P','L'].map((key,i)=>button(`Fire ${key}`,'FIRE',`data-cannon="${i}"`)).join('')}</div>`;break;
 case 'hold-fire':html+=`<div class="trainer-rule">${esc(scene.cue)}</div><div class="trainer-stimulus">${phase==='ENCODE'?'<b>+</b>':`<span class="trainer-shape ${scene.shape} ${scene.color}"></span>`}</div>${phase==='RESPONSE'?button('Respond · Space','PRESS'):''}<p class="field-note">Withhold on non-targets until the window closes.</p>`;break;
 case 'target-number':html+=`<span class="eyebrow">Target</span><div class="trainer-target">${scene.target}</div><div class="trainer-tiles">${move.tiles.map((v:any)=>`<button class="trainer-number-tile" type="button" data-tile="${esc(v.id)}" aria-pressed="false"><b>${v.value}</b><small>${esc(v.expression)}</small></button>`).join('')}</div><p>Select a left tile, an operator, then a right tile.</p><div class="trainer-operators">${['+','-','*','/'].map((op,i)=>`<button class="btn" type="button" data-operator="${op}">${['+','−','×','÷'][i]}</button>`).join('')}</div><p id="trainer-tile-status" role="status"></p><div class="trainer-response-actions">${button('Undo','UNDO')}${button('Reset tiles','RESET')}${button('Skip −1','SKIP','data-skip-lock="true"')}</div>`;break;
 case 'pump':{const cents=scene.cents_per_pump;html+=`<div class="trainer-balloon-wrap"><svg viewBox="0 0 220 260" role="img" aria-label="Balloon with ${move.pumps} safe pumps"><ellipse cx="110" cy="102" rx="${Math.min(95,45+move.pumps)}" ry="${Math.min(100,55+move.pumps)}"/><path d="M110 202L104 213H116ZM110 213Q85 229 110 240T105 258"/></svg><div><span class="eyebrow">At risk this round</span><b class="trainer-target">${((move.pumps||0)*cents/100).toFixed(2)}</b><p>${move.pumps||0} safe pumps · ${cents}¢ each</p></div></div>`;html+=scene.mode==='manual'?`<div class="trainer-response-pair">${button('Pump +10¢','PUMP')}${button('Cash in ↗','CASH_IN')}</div>`:`<form id="trainer-target-form"><label class="field-label">Commit auto-pump target (0–127)<input id="trainer-pump-target" type="number" min="0" max="127" value="20" required/></label><button class="btn primary" type="submit">Commit target ↗</button></form>`;break;}
 case 'skyline':{
  const towers=(stacks:string[][],interactive:boolean)=>`<div class="trainer-towers">${stacks.map((stack:string[],i:number)=>`<${interactive?'button type="button"':'div'} class="trainer-tower" ${interactive?`data-tower="${i}" aria-label="Tower ${i+1}, bottom to top ${stack.join(', ')||'empty'}"`:''}><div>${stack.slice().reverse().map(v=>`<span class="trainer-floor floor-${v}">${esc(v)}</span>`).join('')}</div><small>${i+1} · cap ${scene.capacities[i]}</small></${interactive?'button':'div'}>`).join('')}</div>`;
  html+=`<div class="row"><span class="badge">Minimum ${scene.minimum_moves} moves</span><span class="badge">Your moves: ${move.moves}</span></div><span class="eyebrow">Goal</span>${towers(scene.goal,false)}<span class="eyebrow">Your towers · pick source then destination</span>${towers(move.stacks,true)}<p id="trainer-tower-status" role="status"></p><div class="trainer-response-actions">${button('Undo · counts as a move','UNDO')}${button('Reset · keeps move count','RESET')}</div>`;break;}
 case 'mystery':html+=`<h3 class="trainer-question">Identify the hidden figure.</h3><p>${scene.candidate_count} candidates remain. Feedback tells only how many attributes match.</p><form id="trainer-guess-form"><div class="trainer-fields">${scene.attributes.map((a:any,i:number)=>`<label class="field-label">${esc(a.name)}<select name="attribute-${i}">${a.values.map((v:string)=>`<option>${esc(v)}</option>`).join('')}</select></label>`).join('')}</div><button class="btn primary" type="submit">Test this figure ↗</button></form>${feedback(s)}<div class="table-scroll"><table class="tbl"><thead><tr><th>Guess</th><th>Matching attributes</th></tr></thead><tbody>${(move.guesses||[]).map((v:string[],i:number)=>`<tr><td>${esc(v.join(' / '))}</td><td>${move.counts[i]}</td></tr>`).join('')}</tbody></table></div>`;break;
 case 'sweet-spot':html+=`<p class="field-note">Click a dial when its needle enters the shaded arc. Background clicks and expired dials cost a point.</p><canvas id="trainer-motion" width="800" height="500" aria-label="Rotating timing dials. Click a dial while its needle is within the target arc."></canvas>`;break;
 default:if(scene.quote_prompt)html+=`<h3 class="trainer-question">${esc(scene.quote_prompt)}</h3><form id="trainer-quote-form"><div class="trainer-fields">${numberInput('trainer-bid','Bid','')}${numberInput('trainer-ask','Ask','')}${numberInput('trainer-size','Size','1')}</div><button class="btn primary" type="submit">Submit quote ↗</button></form>`;
 }
 return html;
}
export function paintTrainerMotion(canvas:HTMLCanvasElement,s:any,elapsed:number):void{
 const ctx=canvas.getContext('2d');if(!ctx)return;const width=canvas.width,height=canvas.height;ctx.clearRect(0,0,width,height);ctx.strokeStyle='rgba(31,43,224,.12)';ctx.lineWidth=1;for(let x=0;x<width;x+=40){ctx.beginPath();ctx.moveTo(x,0);ctx.lineTo(x,height);ctx.stroke()}for(let y=0;y<height;y+=40){ctx.beginPath();ctx.moveTo(0,y);ctx.lineTo(width,y);ctx.stroke()}
 ctx.fillStyle='#1f2be0';ctx.strokeStyle='#1f2be0';ctx.lineWidth=2;
 if(s.kind==='sweet-spot')for(const d of s.scene.dials||[]){
  if(elapsed>=d.expires_ms)continue;const rad=(deg:number)=>deg*Math.PI/180;const theta=rad(d.theta0+d.omega*(elapsed-d.spawn_ms)/1000);ctx.beginPath();ctx.lineWidth=20;ctx.strokeStyle='rgba(31,43,224,.28)';ctx.arc(d.x,d.y,d.radius-10,rad(d.target-d.width/2),rad(d.target+d.width/2));ctx.stroke();ctx.lineWidth=2;ctx.strokeStyle='#1f2be0';ctx.beginPath();ctx.arc(d.x,d.y,d.radius,0,Math.PI*2);ctx.stroke();ctx.beginPath();ctx.moveTo(d.x,d.y);ctx.lineTo(d.x+Math.cos(theta)*(d.radius-8),d.y+Math.sin(theta)*(d.radius-8));ctx.stroke();ctx.beginPath();ctx.arc(d.x,d.y,5,0,Math.PI*2);ctx.fill();ctx.fillRect(d.x-d.radius,d.y+d.radius+10,2*d.radius*Math.max(0,(d.expires_ms-elapsed)/(d.expires_ms-d.spawn_ms)),4);
 }
 if(s.kind==='intercept'){
  const a=s.scene,t=Math.max(0,(elapsed-s.onset_ms)/1000),ballY=-20+a.speed*t;ctx.setLineDash([5,8]);ctx.beginPath();ctx.moveTo(a.line_x,0);ctx.lineTo(a.line_x,height);ctx.stroke();ctx.setLineDash([]);
  a.cannons.forEach((c:any,i:number)=>{const cross=a.crossings[i];ctx.strokeStyle='rgba(31,43,224,.28)';ctx.beginPath();ctx.moveTo(c.x,c.y);ctx.lineTo(cross.x,cross.y);ctx.stroke();ctx.strokeStyle='#1f2be0';ctx.font='22px monospace';ctx.textAlign=c.x===0?'left':'right';ctx.fillText(['Q','A','P','L'][i],c.x===0?8:792,c.y-10);ctx.beginPath();ctx.arc(cross.x,cross.y,6,0,Math.PI*2);ctx.stroke()});
  if(!a.occluded||ballY<240){ctx.fillStyle='#1f2be0';ctx.beginPath();ctx.arc(a.line_x,ballY,13,0,Math.PI*2);ctx.fill()}else{ctx.fillStyle='rgba(31,43,224,.06)';ctx.fillRect(a.line_x-27,240,54,height-240)}
  if(a.shot){const shot=a.shot,c=a.cannons[shot.cannon],progress=Math.max(0,Math.min(1,(elapsed-shot.fire_ms)/shot.travel_ms));ctx.fillStyle='#d0613b';ctx.beginPath();ctx.arc(c.x+(shot.x-c.x)*progress,c.y+(shot.y-c.y)*progress,8,0,Math.PI*2);ctx.fill()}
 }
}
