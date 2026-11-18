const sessionPath=/^\/api\/sessions\/(s-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\/(state|action|results|tape)$/;
const publicPaths=new Set(['/api/games','/api/assessments','/api/trainers','/api/trainers/library']);
const replayPaths=new Set(['/api/assessments/replay','/api/trainers/replay']);
function json(data,status=200){return Response.json(data,{status,headers:{'Cache-Control':'no-store'}})}
async function readBody(request,limit){
 if(!request.body)return '';
 const reader=request.body.getReader(),chunks=[];let length=0;
 try{while(true){const {done,value}=await reader.read();if(done)break;length+=value.byteLength;if(length>limit){await reader.cancel();throw new RangeError('Request exceeds 1 MiB')};chunks.push(value)}}finally{reader.releaseLock()}
 const body=new Uint8Array(length);let offset=0;for(const chunk of chunks){body.set(chunk,offset);offset+=chunk.byteLength}return new TextDecoder().decode(body);
}
export function createGateway({fetchGo,revision='local'}){
 const catalogue=new Map();
 return {async fetch(request,env,ctx){
  const url=new URL(request.url),path=url.pathname;let response;
  try{
   if(path==='/healthz'||path==='/api/version')return json({ok:true,revision,runtime:'workers-go',session_schema:'quant-edge-session-v1'});
   if(request.method==='GET'&&publicPaths.has(path)){
    if(!catalogue.has(path))catalogue.set(path,(async()=>{const result=await fetchGo(request,env,ctx);if(!result.ok)throw new Error('Catalogue unavailable');return {body:await result.text(),headers:result.headers}})());
    try{const cached=await catalogue.get(path);response=new Response(cached.body,{headers:cached.headers})}catch(error){catalogue.delete(path);throw error}
   }else if(request.method==='POST'&&replayPaths.has(path))response=await fetchGo(request,env,ctx);
   else if(request.method==='POST'&&path==='/api/sessions'){
    const body=await readBody(request,1<<20),id='s-'+crypto.randomUUID();
    const headers=new Headers(request.headers);headers.set('X-Quant-Session',id);
    response=await env.SESSIONS.getByName(id).fetch(new Request(request.url,{method:'POST',headers,body}));
   }else if(path.startsWith('/api/sessions/')){
    const match=path.match(sessionPath);if(!match)return json({error:'Unknown session route'},404);
    const [,id,action]=match,method=action==='action'?'POST':'GET';if(request.method!==method)return json({error:'Method not allowed'},405);
    const headers=new Headers(request.headers);headers.set('X-Quant-Session',id);
    const body=method==='POST'?await readBody(request,1<<20):undefined;
    response=await env.SESSIONS.getByName(id).fetch(new Request(request.url,{method,headers,body}));
   }else if(path.startsWith('/api/'))return json({error:'Unknown API route'},404);
   else return env.ASSETS.fetch(request);
   const headers=new Headers(response.headers);headers.set('Cache-Control','no-store');headers.set('X-Quant-Revision',revision);
   return new Response(response.body,{status:response.status,headers});
  }catch(error){if(error instanceof RangeError)return json({error:error.message},413);console.error('Quant Practice Worker request failed:',error.message);return json({error:'Service unavailable. Retry the same action.'},503)}
 }};
}
