import {cp,mkdir,access} from 'node:fs/promises';
import {homedir} from 'node:os';
import {join} from 'node:path';
const names=['web-quality-audit','performance','core-web-vitals','accessibility','seo','best-practices'];
const destination=join(process.env.CODEX_HOME||join(homedir(),'.codex'),'skills');
await mkdir(destination,{recursive:true});
for(const name of names){const target=join(destination,name);try{await access(target);console.log(`Already installed: ${name}`);continue}catch{}await cp(new URL(`../codex-skills/${name}/`,import.meta.url),target,{recursive:true,errorOnExist:true,force:false});console.log(`Installed: ${target}`)}
console.log('Skills will be available on your next Codex turn.');
