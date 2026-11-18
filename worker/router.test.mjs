import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createGateway} from './router.mjs';
const uuid='s-01234567-1234-4321-9876-123456789abc';
test('routes creation and subsequent requests to per-session objects; replaces spoofed identity',async()=>{
 const calls=[];const env={SESSIONS:{getByName(id){return {async fetch(req){calls.push([id,req.headers.get('X-Quant-Session'),req.method]);return Response.json({session_id:id})}}}}};
 const app=createGateway({fetchGo:()=>{throw Error('Sessions must not use stateless Wasm')}});
 const created=await app.fetch(new Request('https://test/api/sessions',{method:'POST',body:'{"game":"trainer"}',headers:{'X-Quant-Session':'attacker'}}),env,{});
 assert.equal(created.status,200);assert.match(calls[0][0],/^s-[a-f0-9-]{36}$/);assert.equal(calls[0][0],calls[0][1]);
 await app.fetch(new Request(`https://test/api/sessions/${uuid}/state?player=p1`,{headers:{'X-Quant-Session':'attacker'}}),env,{});assert.deepEqual(calls[1],[uuid,uuid,'GET']);
 assert.equal((await app.fetch(new Request(`https://test/api/sessions/${uuid}/action`),env,{})).status,405);
 assert.equal((await app.fetch(new Request('https://test/api/sessions/s-1/state'),env,{})).status,404);
});
test('catalogue is cached only inside a release; assets and version use the intended routes',async()=>{
 let calls=0;const app=createGateway({revision:'test-sha',fetchGo:async()=>{calls++;return Response.json({profiles:[]})}}),env={ASSETS:{fetch:()=>new Response('home')}};
 for(let i=0;i<2;i++){const res=await app.fetch(new Request('https://test/api/trainers'),env,{});assert.equal(res.headers.get('X-Quant-Revision'),'test-sha');await res.text()};assert.equal(calls,1);
 assert.equal(await (await app.fetch(new Request('https://test/'),env,{})).text(),'home');assert.equal((await (await app.fetch(new Request('https://test/api/version'),env,{})).json()).revision,'test-sha');
});
test('oversized action bodies reject before touching durable storage',async()=>{
 const app=createGateway({fetchGo:async()=>Response.json({})});const res=await app.fetch(new Request('https://test/api/sessions',{method:'POST',body:'x'.repeat((1<<20)+1)}),{},{});assert.equal(res.status,413)
});
