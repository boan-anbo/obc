// Maintainer-only read-only canonical export. End users do not need Node.
import {readFileSync} from 'node:fs';
import {createHash} from 'node:crypto';
const root=process.env.OBC_REFERENCE;
const hash=bytes=>createHash('sha256').update(bytes).digest('hex');
// Ordered transitive local runtime inputs. Names/order/bytes are all retained.
const sourceOrder=['preview-catalog.ts','public-query.ts','search.ts','query-intent.ts','query-ranking.ts','query-plan.ts','author-aggregation.ts','reviewed-author-groups.json','filter-selection.ts','query-results.ts','bibliography-contract.ts'];
const sources=Object.fromEntries(sourceOrder.map(f=>[f,readFileSync(`${root}/src/${f}`,'utf8')]));
const sourceHashes=Object.fromEntries(sourceOrder.map(f=>[f,hash(sources[f])]));
const semanticsSHA256=hash(Buffer.concat(sourceOrder.map(f=>Buffer.from(sources[f]))));
const {buildIndex,validatePreviewCatalog}=await import(`${root}/src/preview-catalog.ts`);
const {authorAggregationVersion}=await import(`${root}/src/author-aggregation.ts`);
const {parsePublicQuery,executePublicQuery}=await import(`${root}/src/public-query.ts`);
const overlayJSON=readFileSync(`${root}/public/agents/author-aggregation.json`,'utf8');
const binding=JSON.parse(readFileSync(`${root}/public/data/download/manifest.json`,'utf8'));
const raw=readFileSync(`${root}/public${binding.catalog}`);
const catalog=validatePreviewCatalog(JSON.parse(raw)),index=buildIndex(catalog);
const queries=['','q=Adams Bodomo','q=Adams B. Bodomo','q=Anthony J. Barbieri-Low','q=Barbieri-Low','q=T’ung-tsu Ch’ü','q=Ch’ü','q=history of computing','q=computing ','q=中国','q=文化大革命','q=late qing','q=1980s','q=published in 2000','q=1980 to 1990出版','q=politics and society','q=the','q=zzzzunfindable','author=adams-bodomo','author=adams-b-bodomo','author=person-adams-bodomo','region=china-all','format=book&workType=study','sort=title','sort=year-new','sort=year-old','subject=politics','period=qing','q=china&limit=3&offset=7','q=decolonization','q=how did women work across China','q=O’Connor','q=政治与社会','q=history histories','q=politics political','q=ΔΗΜΟΣ','q=δῆμος'];
// Derived spellings of one accepted concept remain ONE canonical clue.
for(const q of ['Adams Bodomo 图书馆学 档案学','Adams Bodomo 图书馆学与档案学','Adams Bodomo 语文学 文献学','Adams Bodomo 语文学与文献学'])queries.push(new URLSearchParams({q}).toString());
const combinations=[...new Set(Object.values(catalog.taxonomy).flat().flatMap(t=>[t.zh,t.zht]).filter(n=>n.length<=18&&n.split(/[与與和及、]/u).filter(Boolean).length>1))].slice(0,12);
for(const joined of combinations){const split=joined.split(/[与與和及、]/u).filter(Boolean).join(' ');for(const q of [joined,split]){queries.push(new URLSearchParams({q}).toString());queries.push(new URLSearchParams({q:'Adams Bodomo '+q}).toString());queries.push(new URLSearchParams({q,author:'person-adams-bodomo'}).toString());}}
const cases=queries.map(q=>{const p=new URLSearchParams(q);p.set('limit',q.includes('limit=')?'3':'100');p.set('facets','period,format,subject');return {params:Object.fromEntries(p),result:executePublicQuery(index,parsePublicQuery(p),binding.snapshot,true)};});
const titleOrder=catalog.works.map((_,i)=>i).sort((a,b)=>catalog.works[a].title.localeCompare(catalog.works[b].title,'en')||catalog.works[a].id.localeCompare(catalog.works[b].id,'en'));
for(const f of sourceOrder)if(readFileSync(`${root}/src/${f}`,'utf8')!==sources[f])throw Error(`Reference input changed during export: ${f}`);
const generation={node:process.version,icu:process.versions.icu,unicode:process.versions.unicode,collation:new Intl.Collator('en').resolvedOptions(),sourceOrder,sourceHashes,sourceDigestAlgorithm:'SHA256 of UTF-8 source bytes concatenated in sourceOrder'};
process.stdout.write(JSON.stringify({generation,sources,retrieval:{schema:'openbibliographies.retrieval.v1',catalogSHA256:hash(raw),authorVersion:authorAggregationVersion,semanticsSHA256,authorOverlayJSON:overlayJSON,authorOverlaySHA256:hash(overlayJSON),titleOrder},cases}));
