// Same normal en localeCompare/title+ID tie as export-reference.mjs. No query
// fixture or accepted source is changed by this data-only export.
import fs from 'node:fs';
const catalog=JSON.parse(fs.readFileSync(process.argv[2],'utf8'));
const order=catalog.works.map((_,i)=>i).sort((a,b)=>catalog.works[a].title.localeCompare(catalog.works[b].title,'en')||catalog.works[a].id.localeCompare(catalog.works[b].id,'en'));
process.stdout.write(JSON.stringify(order)+'\n');
