// Point this at a staging deployment before promoting the same commit.
import assert from 'node:assert/strict';
const base=process.env.QUANT_BASE_URL;
if(!base)throw Error('Set QUANT_BASE_URL to the deployed Worker URL in this GitHub environment.');
const url=new URL(base);if(!['https:','http:'].includes(url.protocol))throw Error('QUANT_BASE_URL must be HTTP(S)');
async function request(path,body){const response=await fetch(new URL(path,url),{method:body?'POST':'GET',headers:body?{'Content-Type':'application/json'}:undefined,body:body?JSON.stringify(body):undefined,signal:AbortSignal.timeout(30000)});const text=await response.text();assert.equal(response.ok,true,`${path}: ${response.status} ${text.slice(0,300)}`);return JSON.parse(text)}
const version=await request('/api/version');if(process.env.QUANT_EXPECTED_REVISION){
 // A fresh deploy can take a few seconds to propagate to the serving edge.
 let observed=version.revision;for(let attempt=1;observed!==process.env.QUANT_EXPECTED_REVISION&&attempt<=6;attempt++){await new Promise(r=>setTimeout(r,5000));observed=(await request('/api/version')).revision}
 assert.equal(observed,process.env.QUANT_EXPECTED_REVISION,'Stale Worker revision');
}
assert.equal((await request('/api/trainers')).profiles.length,22);
assert.equal((await request('/api/assessments')).profiles.length,25);
for(const [game,config] of [['trainer',{trainer_id:'math'}],['assessment',{profile_id:'mako-aptitude',mode:'preview'}]]){
 const session=await request('/api/sessions',{game,players:['smoke'],seed:42,config});
 const state=await request(`/api/sessions/${session.session_id}/state?player=smoke`);assert.equal(state.game,game);
 if(game==='trainer')for(const action of ['BEGIN','END_SESSION']){const result=await request(`/api/sessions/${session.session_id}/action`,{player_id:'smoke',action_id:`smoke-${action}`,action,payload:{elapsed_ms:0}});assert.equal(result.output.accepted,true)}
}
for(const path of ['/','/games/','/assessment/','/trainer/','/favicon.svg']){const response=await fetch(new URL(path,url),{signal:AbortSignal.timeout(30000)});assert.equal(response.ok,true,`${path}: ${response.status}`)}
console.log(`Verified ${base} at revision ${version.revision}`);
