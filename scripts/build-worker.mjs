import {execFileSync} from 'node:child_process';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
const backend=fileURLToPath(new URL('../backend/',import.meta.url));
const output=fileURLToPath(new URL('../worker/build/',import.meta.url));
await mkdir(output,{recursive:true});
execFileSync('go',['run','github.com/syumai/workers-go/cmd/workers-assets-gen','-mode=go','-durable-objects=GameSession','-o',output],{cwd:backend,stdio:'inherit'});
execFileSync('go',['build','-trimpath','-ldflags=-s -w','-o',`${output}/app.wasm`,'./cmd/worker'],{cwd:backend,stdio:'inherit',env:{...process.env,GOOS:'js',GOARCH:'wasm'}});
// A shared release identifier is visible to deployment smoke checks.
const revision=process.env.GITHUB_SHA||process.env.QUANT_REVISION||'local';
await writeFile(new URL('../worker/build/revision.mjs',import.meta.url),`export default ${JSON.stringify(revision)};\n`);
const wasm=await readFile(`${output}/app.wasm`);console.log(`Go Worker built: ${(wasm.length/1024/1024).toFixed(2)} MiB uncompressed; revision ${revision}`);
