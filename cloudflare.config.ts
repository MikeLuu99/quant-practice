import {bindings,defineConfig,exports as workerExports} from 'cf/config';

export default defineConfig(({mode})=>{
 const production=mode==='production';
 const name=production?'quant-practice':'quant-practice-staging';
 return {worker:{
  name,entrypoint:'./worker/index.mjs',compatibilityDate:'2026-11-17',
  workersDev:true,
  assets:{runWorkerFirst:['/api/*','/healthz'],htmlHandling:'auto-trailing-slash',notFoundHandling:'404-page'},
  env:{SESSIONS:bindings.durableObject({worker:name,exportName:'GameSession'}),ASSETS:bindings.assets()},
  exports:{GameSession:workerExports.durableObject({storage:'sqlite'})},
  observability:{enabled:true},
 }};
});
