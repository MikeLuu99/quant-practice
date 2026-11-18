import { companyLogo } from "./company-brands";
import profiles from "../../../backend/internal/games/assessment_profiles.json";
import { api, createSession, getState, getResults, getTape, sendAction, nextActionId, fmtNum } from "./api";
import { escapeHTML as esc, fmtClock } from "./logic";
import { renderAssessmentAsset, renderAssessmentInputs, responseSummary, type AssessmentQuestion } from "./assessment-ui";

type Profile = typeof profiles[number];
const $ = <T extends HTMLElement = HTMLElement>(id:string) => document.getElementById(id) as T;
const input = (id:string) => $<HTMLInputElement>(id);
const select = (id:string) => $<HTMLSelectElement>(id);
const url = new URL(location.href);
let profile:Profile = profiles.find(p=>p.id===url.searchParams.get("profile")) || profiles[0];
let sid = url.searchParams.get("s") || "";
let player = url.searchParams.get("me") || "p1";
let state:any = null;
let question:AssessmentQuestion | null = null;
let renderedID = "";
let navSignature = "";
let generation = 0;
let readEpoch = 0;
let revision = 0;
let finished = false;
let reviewLoaded = false;
let navigating = false;
let writeCount = 0;
let pollTimer:ReturnType<typeof setTimeout> | undefined;
let saveTimer:ReturnType<typeof setTimeout> | undefined;
let writeChain:Promise<any> = Promise.resolve();
const gameNames:Record<string,string> = {math:"Math Challenge",fruit:"Fruit Market",prob:"Probability Betting",nextcard:"Next Card Betting",cards:"Market of Cards",fermi:"Fermi Questions",makeme:"Make Me a Market",etf:"ETF Trading"};

function mode():string {return document.querySelector<HTMLInputElement>('input[name="mode"]:checked')!.value;}
function error(text:string):void {$('assessment-error').textContent=text;}
function answerError(text:string):void {$('attempt-answer-error').textContent=text;}
function profileDuration(p:Profile,preview=false):string {return p.timer_mode==='per_question'?`${p.item_time_ms/1000}s per item · original timer`:`${(preview?p.preview_duration_ms:p.duration_ms)/60000} min`;}
function populateProfiles():void {
  const company=select('assessment-company').value;
  const list=profiles.filter(p=>p.company_id===company);
  select('assessment-profile').replaceChildren(...list.map(p=>{const o=document.createElement('option');o.value=p.id;o.textContent=`${p.title} · ${p.variant}`;return o;}));
  if(!list.some(p=>p.id===profile.id)) profile=list[0];
  select('assessment-profile').value=profile.id;
  configureProfile();
}
function chooseProfile(id:string,scroll=false):void {
  const chosen=profiles.find(p=>p.id===id);if(!chosen)return;
  profile=chosen;select('assessment-company').value=chosen.company_id;populateProfiles();
  url.searchParams.set('profile',id);history.replaceState(null,'',url.toString());
  if(scroll)$('assessment-start-form').scrollIntoView({behavior:'smooth',block:'start'});
}
function configureProfile():void {
  input('custom-count').value=String(profile.question_count);
  input('custom-duration').value=String(profile.duration_ms/60000||5);
  input('custom-item-time').value=String(profile.item_time_ms/1000||45);
  select('custom-navigation').value=profile.navigation;
  select('custom-navigation').disabled=profile.timer_mode==='per_question';
  select('custom-response').value=profile.response_mode;
  for(const option of Array.from(select('custom-response').options)) {
    option.disabled=profile.response_mode==='interval'?option.value!=='interval':option.value==='interval'||(option.value==='token'&&!profile.engines.includes('sequences'));
  }
  select('custom-family').innerHTML='<option value="">Complete profile mix</option>'+[...new Set(profile.families)].map(f=>`<option value="${esc(f)}">${esc(f.replaceAll('_',' '))}</option>`).join('');
  $('custom-duration-label').hidden=profile.timer_mode==='per_question';
  $('custom-item-label').hidden=profile.timer_mode!=='per_question';
  $('assessment-profile-info').innerHTML=`${companyLogo(profile.company_id,profile.company)}<h2>${esc(profile.title)}</h2><div class="profile-variant">${esc(profile.variant)}</div><dl class="profile-facts"><div><dt>Full test</dt><dd>${profile.question_count} questions / ${profileDuration(profile)}</dd></div><div><dt>Preview</dt><dd>${profile.preview_count} questions / ${profileDuration(profile,true)}</dd></div><div><dt>Navigation</dt><dd>${profile.navigation==='revisable'?'Revisit & revise':'Forward only'}</dd></div><div><dt>Scoring</dt><dd>${profile.response_mode==='interval'?'Interval loss · lower is better':'+1 correct / 0 wrong or blank'}</dd></div></dl><p class="field-note">Aids: ${esc(profile.aids)}</p><details><summary>Format details & sources</summary><p>${esc(profile.notes)}</p><p class="field-note">Original question bank. Navigation: ${esc(profile.navigation_origin)}. Scoring: ${esc(profile.score_origin)}.</p><b>Practice rules</b><ul>${profile.proposed.map(n=>`<li>${esc(n)}</li>`).join('')}</ul><b>Unconfirmed source details</b><ul>${profile.unknown.map(n=>`<li>${esc(n)}</li>`).join('')}</ul><a href="${esc(profile.source)}" target="_blank" rel="noreferrer">Source ↗</a></details>${profile.overlap.length?`<details class="profile-overlap"><summary>Related games</summary><div class="row">${profile.overlap.map(slug=>`<a class="btn sm" href="/games/${esc(slug)}/">${esc(gameNames[slug]||slug)} ↗</a>`).join('')}</div></details>`:''}`;
  updateMode();
}
function updateMode():void {
  const m=mode();$('assessment-custom').hidden=m!=='custom';
  const count=m==='custom'?Number(input('custom-count').value):m==='preview'?profile.preview_count:profile.question_count;
  const timing=m==='custom'?(profile.timer_mode==='per_question'?`${input('custom-item-time').value}s / item`:`${input('custom-duration').value} min`):profileDuration(profile,m==='preview');
  $('assessment-start-summary').textContent=`${count} questions · ${timing}`;
}
select('assessment-company').addEventListener('change',populateProfiles);
select('assessment-profile').addEventListener('change',()=>chooseProfile(select('assessment-profile').value));
document.querySelectorAll('input[name="mode"]').forEach(r=>r.addEventListener('change',updateMode));
$('assessment-custom').addEventListener('input',updateMode);
document.querySelectorAll<HTMLButtonElement>('[data-profile]').forEach(button=>button.addEventListener('click',()=>chooseProfile(button.dataset.profile!,true)));
function filterCatalogue():void {
  const search=input('assessment-search').value.toLowerCase().trim();const family=select('assessment-engine-filter').value;let count=0;
  document.querySelectorAll<HTMLElement>('.firm-assessment').forEach(card=>{const visible=(!search||card.dataset.search!.includes(search))&&(!family||card.dataset.engines!.split(' ').includes(family));card.hidden=!visible;if(visible)count++;});
  document.querySelectorAll<HTMLElement>('.firm-card').forEach(card=>card.hidden=!card.querySelector('.firm-assessment:not([hidden])'));
  $('assessment-filter-count').textContent=`${count} profile${count===1?'':'s'}`;$('assessment-no-matches').hidden=count>0;
}
input('assessment-search').addEventListener('input',filterCatalogue);select('assessment-engine-filter').addEventListener('change',filterCatalogue);
chooseProfile(profile.id);

function stopPolling():void {if(pollTimer)clearTimeout(pollTimer);pollTimer=undefined;}
function stopSaving():void {if(saveTimer)clearTimeout(saveTimer);saveTimer=undefined;}
function schedulePoll():void {
  stopPolling();if(!sid||(finished&&reviewLoaded))return;
  pollTimer=setTimeout(async()=>{if(writeCount||navigating){schedulePoll();return;}try{await refresh();}catch{$('attempt-connection-status').textContent='Connection interrupted · retrying';}schedulePoll();},750);
}
function showSession():void {
  $('assessment-setup').hidden=true;$('assessment-intro').hidden=true;$('assessment-session').hidden=false;document.body.classList.add('is-assessing');
  url.searchParams.set('s',sid);url.searchParams.set('me',player);url.searchParams.set('profile',profile.id);history.replaceState(null,'',url.toString());
}
function updateNavigator():void {
  const signature=JSON.stringify([state.index,state.saved_question_ids,state.committed_question_ids,finished]);if(signature===navSignature)return;navSignature=signature;
  $('assessment-question-nav').innerHTML=Array.from({length:state.question_count},(_,i)=>{const id=`q${String(i+1).padStart(3,'0')}`;const answered=state.saved_question_ids.includes(id);return `<button type="button" class="question-index ${answered?'is-saved':''} ${state.index===i?'is-current':''}" data-index="${i}" ${state.navigation!=='revisable'||finished?'disabled':''} ${state.index===i?'aria-current="step"':''} aria-label="Question ${i+1}, ${answered?'answered':'unanswered'}">${i+1}</button>`;}).join('');
  $('assessment-question-nav').querySelectorAll<HTMLButtonElement>('button').forEach(b=>b.addEventListener('click',()=>navigate('JUMP_TO',Number(b.dataset.index))));
}
async function refresh():Promise<void> {
  if(!sid)return;
  const epoch=++readEpoch;const ownGeneration=generation;const data=await getState(sid,player);
  if(epoch!==readEpoch||ownGeneration!==generation)return;
  if(data.game!=='assessment')throw new Error('This link belongs to a trading game. Open it from the games page.');
  state=data.state;finished=data.finished;profile=profiles.find(p=>p.id===state.profile.id)||profile;
  $('attempt-company').textContent=`${profile.company} / ${state.config.mode} / ${profile.variant}`;
  $('attempt-title').textContent=profile.title;
  if ($('attempt-logo').dataset.company !== profile.company_id) {
    $('attempt-logo').innerHTML=companyLogo(profile.company_id,profile.company);
    $('attempt-logo').dataset.company=profile.company_id;
  }
  $('attempt-clock').textContent=fmtClock(finished?0:state.time_remaining_ms);
  $('attempt-timer-label').textContent=finished?'Assessment complete':state.timer_mode==='per_question'?'This question':'Time remaining';
  $('attempt-count').textContent=`${state.saved_question_ids.length} / ${state.question_count} answered`;
  const progress=$<HTMLProgressElement>('attempt-progress');progress.max=state.question_count;progress.value=state.saved_question_ids.length;
  $('attempt-nav-policy').textContent=state.navigation==='revisable'?'You can revisit answers before submitting.':'Forward only. Submitted answers are locked.';
  updateNavigator();
  document.querySelector<HTMLElement>(".assessment-workspace")!.hidden=finished;
  document.querySelector<HTMLElement>(".assessment-submit-row")!.hidden=finished;
  if(!finished&&state.question.question_id!==renderedID) {
    stopSaving();question=state.question;renderedID=question!.question_id;revision=0;
    $('attempt-prompt').textContent=question!.prompt;
    $('attempt-question-family').textContent=question!.family.replaceAll('_',' ');
    $('attempt-question-number').textContent=`${state.index+1} / ${state.question_count}`;
    $('attempt-assets').innerHTML=renderAssessmentAsset(question!);
    $('attempt-inputs').innerHTML=renderAssessmentInputs(question!,state.saved_response);
    $('attempt-save-status').textContent=Object.keys(state.saved_response||{}).length?'Answer saved':'No answer yet';answerError('');
    $('attempt-inputs').querySelector<HTMLInputElement>('input:not([type=radio])')?.focus({preventScroll:true});
  }
  $<HTMLButtonElement>('attempt-previous').hidden=state.navigation!=='revisable';
  $<HTMLButtonElement>('attempt-previous').disabled=state.index===0||finished;
  $<HTMLButtonElement>('attempt-next').disabled=finished;
  $<HTMLButtonElement>('attempt-skip').disabled=finished;
  $<HTMLButtonElement>('attempt-next').textContent=state.index+1===state.question_count?(state.navigation==='revisable'?'Save answer ✓':'Save & finish ↗'):'Save & next →';
  $<HTMLButtonElement>('attempt-skip').textContent=state.navigation==='revisable'&&state.index+1===state.question_count?'Clear answer':'Skip question →';
  $('attempt-connection-status').textContent=finished?'Session complete · solutions available':'Connected · server timer';
  $('attempt-submit').hidden=finished;
  if(finished) {
    stopSaving();$('attempt-save-status').textContent='Finalized';
    $('assessment-answer-form').querySelectorAll<HTMLInputElement|HTMLButtonElement|HTMLTextAreaElement>('input,button,textarea').forEach(el=>el.disabled=true);
    if(!reviewLoaded){const result=await getResults(sid);if(ownGeneration!==generation)return;renderResults(result.results);reviewLoaded=true;$<HTMLButtonElement>('attempt-export').disabled=false;}
  }
}
const rejection:Record<string,string>={INVALID_FORMAT:'Enter a valid answer. Numeric fields accept decimals, fractions, and scientific notation; intervals need two positive bounds in order.',STALE_QUESTION:'That question has expired or changed. Your answer was not applied to the next question.',NAVIGATION_LOCKED:'This format does not allow backward navigation.',LATE:'The assessment deadline has passed.',MARKET_CLOSED:'The assessment is complete.'};
async function write(action:string,payload:any):Promise<any> {
  const ownGeneration=generation;const ownSID=sid;const ownPlayer=player;readEpoch++;writeCount++;
  const task=writeChain.then(async()=>{
    if(ownGeneration!==generation)return {accepted:false,reason:'ATTEMPT_CHANGED'};
    return sendAction(ownSID,{action_id:nextActionId(),player_id:ownPlayer,action,payload});
  });
  writeChain=task.catch(()=>{});
  try{return await task;}finally{writeCount--;}
}
function currentResponse():any {
  if(!question)return {};
  if(question.response_type==='single_choice')return {answer:document.querySelector<HTMLInputElement>('#attempt-inputs input[name="choice"]:checked')?.value||''};
  if(question.response_type==='interval')return {low:input('interval-low').value.trim(),high:input('interval-high').value.trim(),reasoning:$<HTMLTextAreaElement>('interval-reasoning').value};
  return {answer:input('assessment-answer').value.trim()};
}
function responseBlank(response:any):boolean {return !response.answer&&!response.low&&!response.high&&!response.reasoning;}
async function saveCurrent(report=true):Promise<boolean> {
  if(!question||finished)return false;
  stopSaving();const id=question.question_id;const response=currentResponse();const ownRevision=revision;const ownGeneration=generation;
  $('attempt-save-status').textContent='Saving…';
  try {
    const out=await write(responseBlank(response)?'CLEAR_ANSWER':'SAVE_ANSWER',{question_id:id,response});
    if(ownGeneration!==generation||id!==renderedID)return false;
    if(!out.accepted){$('attempt-save-status').textContent='Draft not saved';if(report)answerError(rejection[out.reason]||out.reason);return false;}
    if(ownRevision===revision)$('attempt-save-status').textContent=responseBlank(response)?'Answer cleared':'Saved to server';
    if(report)answerError('');return true;
  } catch(e){$('attempt-save-status').textContent='Not saved · connection interrupted';if(report)answerError((e as Error).message);return false;}
}
$('attempt-inputs').addEventListener('input',()=>{revision++;answerError('');$('attempt-save-status').textContent='Unsaved draft';stopSaving();saveTimer=setTimeout(()=>void saveCurrent(false),450);});
async function navigate(action:string,index?:number,skip=false):Promise<void> {
  if(navigating||finished||!question)return;
  navigating=true;stopSaving();const id=question.question_id;
  $<HTMLButtonElement>('attempt-next').disabled=true;
  try {
    if(skip){const out=await write('CLEAR_ANSWER',{question_id:id});if(!out.accepted){answerError(rejection[out.reason]||out.reason);return;}}
    else if(!await saveCurrent())return;
    const lastRevisable=action==='NEXT'&&state.navigation==='revisable'&&state.index+1===state.question_count;
    if(!lastRevisable){const out=await write(action,{question_id:id,index});if(!out.accepted)answerError(rejection[out.reason]||out.reason);}
    await refresh();
  } catch(e){answerError((e as Error).message);}
  finally{navigating=false;$<HTMLButtonElement>('attempt-next').disabled=finished;schedulePoll();}
}
$('assessment-answer-form').addEventListener('submit',ev=>{ev.preventDefault();void navigate('NEXT');});
$('attempt-previous').addEventListener('click',()=>void navigate('PREVIOUS'));
$('attempt-skip').addEventListener('click',()=>void navigate('NEXT',undefined,true));
$('attempt-submit').addEventListener('click',()=>void navigate('SUBMIT_TEST'));
$('assessment-answer-form').addEventListener('keydown',ev=>{
  if(question?.response_type!=='single_choice'||ev.altKey||ev.ctrlKey||ev.metaKey)return;
  if(/^[1-5]$/.test(ev.key)){const radio=$('attempt-inputs').querySelectorAll<HTMLInputElement>('input[type=radio]')[Number(ev.key)-1];if(radio){radio.checked=true;radio.dispatchEvent(new Event('input',{bubbles:true}));ev.preventDefault();}}
});
$('assessment-start-form').addEventListener('submit',async ev=>{
  ev.preventDefault();error('');const button=$<HTMLButtonElement>('assessment-start');button.disabled=true;
  try {
    const raw=input('assessment-seed').value.trim();const seed=raw?Number(raw):undefined;
    if(seed!==undefined&&(!Number.isSafeInteger(seed)||seed<=0))throw new Error('Use a positive whole number for the challenge seed.');
    player=input('assessment-player').value.trim();if(!player)throw new Error('Enter a player name.');
    const config:any={profile_id:profile.id,mode:mode()};
    if(config.mode==='custom') {
      config.question_count=Number(input('custom-count').value);config.navigation=select('custom-navigation').value;config.response_mode=select('custom-response').value;config.family=select('custom-family').value;config.wrong_points=Number(select('custom-wrong').value);
      if(profile.timer_mode==='per_question')config.item_time_ms=Math.round(Number(input('custom-item-time').value)*1000);else config.duration_ms=Math.round(Number(input('custom-duration').value)*60000);
      if(!Number.isInteger(config.question_count)||config.question_count<1)throw new Error('Choose at least one question.');
    }
    generation++;stopPolling();stopSaving();finished=false;reviewLoaded=false;renderedID='';navSignature='';question=null;
    $('assessment-results').hidden=true;$<HTMLButtonElement>('attempt-export').disabled=true;
    const data=await createSession('assessment',[player],config,seed);sid=data.session_id;
    showSession();await refresh();schedulePoll();
  } catch(e){error((e as Error).message);$('assessment-setup').hidden=false;$('assessment-intro').hidden=false;$('assessment-session').hidden=true;}
  finally{button.disabled=false;}
});
function renderResults(result:any):void {
  const interval=result.score_kind==='interval_loss';
  const score=interval?(result.mean_interval_loss==null?'—':fmtNum(result.mean_interval_loss,3)):`${result.raw_score} / ${result.maximum_score}`;
  $('assessment-results').hidden=false;
  $('assessment-results').innerHTML=`<div class="assessment-results-head"><span class="eyebrow">Your assessment review / ${esc(result.score_rule_origin)}</span><h2>${interval?'How wide was your uncertainty?':'Every answer, explained.'}</h2></div><div class="stat-strip"><div class="stat"><span class="eyebrow">${interval?'Mean interval loss ↓':'Raw score'}</span><span class="stat-value">${esc(score)}</span></div><div class="stat"><span class="eyebrow">${interval?'Coverage':'Attempted accuracy'}</span><span class="stat-value">${fmtNum((interval?result.coverage_rate:result.accuracy_attempted)*100,1)}%</span></div><div class="stat"><span class="eyebrow">Completion</span><span class="stat-value">${result.attempted} / ${result.review.length}</span></div></div><p class="field-note">${interval?'Lower interval loss is better. A single interval cannot establish 80% long-run calibration. Reasoning is preserved for human review.':`${result.correct} correct · ${result.wrong} wrong · ${result.blank} unanswered. No employer pass mark or population percentile is inferred.`} Time used: ${fmtClock(result.elapsed_ms)}.</p><div class="assessment-topic-results">${Object.entries(result.topics).map(([family,v]:any)=>`<div><span>${esc(family.replaceAll('_',' '))}</span><span class="mono">${interval?`${v.total} interval question(s)`:`${v.correct} / ${v.total} correct`}</span></div>`).join('')}</div><div class="assessment-review">${result.review.map((r:any,i:number)=>`<details><summary><span class="review-status">${esc(r.status)}</span><span>${i+1}. ${esc(r.question.prompt)}</span><span class="mono">${fmtClock(r.time_spent_ms)}</span></summary><div class="assessment-review-body">${renderAssessmentAsset(r.question)}<div class="review-answers"><p><b>Your response</b><span>${esc(responseSummary(r.response,r.question))}</span></p><p><b>${interval?'Frozen answer':'Correct answer'}</b><span>${esc(r.correct_answer)}</span></p></div>${interval&&r.interval_loss!=null?`<p>Width ${fmtNum(r.interval_width,4)} · Loss ${fmtNum(r.interval_loss,4)} · ${r.covered?'Covered':'Missed'} the frozen answer</p>`:''}<p>${esc(r.explanation)}</p>${r.response?.reasoning?`<blockquote>${esc(r.response.reasoning)}</blockquote>`:''}</div></details>`).join('')}</div>`;
}
$('attempt-export').addEventListener('click',async()=>{
  try{const tape=await getTape(sid);const href=URL.createObjectURL(new Blob([JSON.stringify(tape,null,2)],{type:'application/json'}));const a=document.createElement('a');a.href=href;a.download=`${profile.id}-${sid}.json`;a.click();URL.revokeObjectURL(href);}catch(e){answerError((e as Error).message);}
});
$('attempt-new').addEventListener('click',()=>{
  generation++;readEpoch++;stopPolling();stopSaving();sid='';state=null;question=null;finished=false;renderedID='';navSignature='';reviewLoaded=false;
  $('assessment-session').hidden=true;$('assessment-setup').hidden=false;$('assessment-intro').hidden=false;document.body.classList.remove('is-assessing');
  url.searchParams.delete('s');url.searchParams.delete('me');history.replaceState(null,'',url.toString());chooseProfile(profile.id);
});
input('assessment-replay-file').addEventListener('change',async()=>{
  const file=input('assessment-replay-file').files?.[0];if(!file)return;
  $('assessment-replay-status').textContent='Verifying frozen tape…';
  try{if(file.size>5*1024*1024)throw new Error('Tape must be at most 5 MiB.');const data=await api('/assessments/replay',{method:'POST',body:await file.text()});$('assessment-replay-status').textContent=`Verified: ${data.results.assessment_id}. Transitions, receipts, and final results match the recording.`;}catch(e){$('assessment-replay-status').textContent=`Replay failed: ${(e as Error).message}`;}
});
if(sid){showSession();refresh().then(schedulePoll).catch(e=>{error(`Could not restore attempt: ${e.message}`);$('assessment-session').hidden=true;$('assessment-setup').hidden=false;$('assessment-intro').hidden=false;});}
window.addEventListener('pagehide',()=>{stopPolling();stopSaving();readEpoch++;});
window.addEventListener('pageshow',ev=>{if(ev.persisted&&sid){void refresh().then(schedulePoll);}});
