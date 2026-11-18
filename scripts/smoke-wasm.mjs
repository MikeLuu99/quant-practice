// Exercise the real generated workers-go runtime in Node's VM. SQLite and alarm
// adapters model storage only; this does not replace workerd/staging validation.
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {DatabaseSync} from 'node:sqlite';
import assert from 'node:assert/strict';
const root=fileURLToPath(new URL('../',import.meta.url));
const sandbox={Object,Array,Promise,String,Number,Boolean,Symbol,BigInt,Map,Set,WeakMap,Error,TypeError,RangeError,Reflect,Math,JSON,Date,ReadableStream,WritableStream,TransformStream,Uint8ClampedArray,Float32Array,Float64Array,Int32Array,console,crypto,performance,TextEncoder,TextDecoder,Request,Response,Headers,URL,URLSearchParams,AbortController,Uint8Array,ArrayBuffer,DataView,WebAssembly,fetch,queueMicrotask,
 setTimeout:(fn,ms,...args)=>{const timer=setTimeout(fn,ms,...args);timer.unref();return timer},clearTimeout,
};
const context=vm.createContext(sandbox);const cache=new Map();
async function load(path){
 if(cache.has(path))return cache.get(path);
 if(path.startsWith('cloudflare:')){const names={
  'cloudflare:sockets':{connect(){throw Error('Network sockets are outside this smoke test')}},
  'cloudflare:email':{EmailMessage:class{}},
  'cloudflare:workflows':{NonRetryableError:class extends Error{}},
  'cloudflare:workers':{WorkflowEntrypoint:class{},WorkerEntrypoint:class{}},
 }[path];if(!names)throw Error('Unsupported test module '+path);const mod=new vm.SyntheticModule(Object.keys(names),function(){for(const [key,value] of Object.entries(names))this.setExport(key,value)},{context,identifier:path});cache.set(path,mod);return mod}
 if(path.endsWith('.wasm')){const binary=await readFile(path),compiled=await WebAssembly.compile(binary);const mod=new vm.SyntheticModule(['default'],function(){this.setExport('default',compiled)},{context,identifier:path});cache.set(path,mod);return mod}
 const code=await readFile(path,'utf8'),mod=new vm.SourceTextModule(code,{context,identifier:path});cache.set(path,mod);await mod.link((specifier,parent)=>load(specifier.startsWith('cloudflare:')?specifier:resolve(dirname(parent.identifier),specifier)));return mod;
}
const module=await load(resolve(root,'worker/index.mjs'));await module.evaluate();
const {GameSession}=module.namespace;
const database=new DatabaseSync(':memory:');let alarm=null;
const state={storage:{sql:{exec(query,...bindings){const stmt=database.prepare(query),columns=stmt.columns().map(c=>c.name);const rows=columns.length?stmt.all(...bindings):[];if(!columns.length)stmt.run(...bindings);return {columnNames:columns,toArray:()=>rows,rowsRead:rows.length,rowsWritten:columns.length?0:1}}},async setAlarm(value){alarm=value},async deleteAlarm(){alarm=null}},waitUntil(){}};
let room=new GameSession(state,{});const id='s-wasm';
async function request(path,body){const started=performance.now();const res=await room.fetch(new Request('https://test'+path,{method:body?'POST':'GET',headers:{'Content-Type':'application/json','X-Quant-Session':id},body:body?JSON.stringify(body):undefined}));const text=await res.text();if(!res.ok)throw Error(`${res.status}: ${text}`);return {data:JSON.parse(text),ms:performance.now()-started}}
const created=await request('/api/sessions',{game:'trainer',players:['alice'],seed:42,config:{trainer_id:'math'}});assert.equal(created.data.session_id,id);
let actionNumber=0;let elapsed=0;
async function action(action,payload={}){return request(`/api/sessions/${id}/action`,{action_id:`a-${++actionNumber}`,player_id:'alice',action,payload:{elapsed_ms:elapsed,...payload}})}
assert.equal((await action('BEGIN')).data.output.accepted,true);
let observed=(await request(`/api/sessions/${id}/state?player=alice`)).data.state;const trial=observed.trial_id;
assert.equal((await action('PRESENT',{trial_id:trial})).data.output.accepted,true);
const answered=await action('ANSWER',{trial_id:trial,answer:'0'});assert.equal(answered.data.output.accepted,true);
const before=(await request(`/api/sessions/${id}/state?player=alice`)).data.state;assert.equal(before.completed,1);assert.ok(alarm!=null);
// Eviction: construct a new durable instance over the same SQLite journal.
room=new GameSession(state,{});const after=(await request(`/api/sessions/${id}/state?player=alice`)).data.state;assert.equal(after.completed,1);assert.equal(after.score,before.score);assert.equal(after.feedback.response,'0');
const duplicate=await request(`/api/sessions/${id}/action`,{action_id:`a-${actionNumber}`,player_id:'alice',action:'ANSWER',payload:{elapsed_ms:0,trial_id:trial,answer:'0'}});assert.equal(duplicate.data.output.reason,'DUPLICATE');
assert.equal((await action('END_SESSION')).data.output.accepted,true);assert.equal(alarm,null);
const tape=(await request(`/api/sessions/${id}/tape`)).data;assert.equal(tape.runtime_version,'trainer-runtime-v1');
const replay=await module.namespace.default.fetch(new Request('https://test/api/trainers/replay',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(tape)}),{},{});assert.equal(replay.status,200);assert.equal((await replay.json()).verified,true);
console.log(JSON.stringify({verified:true,wasm_creation_ms:Math.round(created.ms),warm_answer_ms:Math.round(answered.ms),journal_rows:database.prepare('SELECT count(*) AS n FROM session_events').get().n,restored_score:after.score},null,2));
database.close();
