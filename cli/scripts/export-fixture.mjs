// Maintainer-only synthetic conformance fixture, using existing query authority.
import {createHash} from 'node:crypto';
const root=process.env.OBC_REFERENCE;
const {fixtureCatalog}=await import(`${root}/src/test-fixtures/public-preview.ts`);
const {buildIndex,validatePreviewCatalog}=await import(`${root}/src/preview-catalog.ts`);
const {parsePublicQuery,executePublicQuery}=await import(`${root}/src/public-query.ts`);
const hash=s=>createHash('sha256').update(s).digest('hex');
const catalog=fixtureCatalog();const base=catalog.works[0];
for(const [id,name] of [['barbieri','Anthony J. Barbieri-Low'],['chu','T’ung-tsu Ch’ü'],['oconnor',"Caitlin O’Connor"]]) {catalog.works.push({...base,id,title:'Administrative practice '+id,creators:[{name,role:'著'}]});catalog.works.push({...base,id:id+'-incidental',title:'A discussion of '+name,creators:[{name:'Someone Else',role:'editor'}]});}
catalog.works.push({...base,id:'greek',title:'Political community',creators:[{name:'Δῆμος',role:'author'}]});
catalog.taxonomy.subject.push({id:'library-archives',en:'Library and archival science',zh:'图书馆学与档案学',zht:'圖書館學與檔案學',parents:[]});
catalog.works.push({...base,id:'archive-science',title:'Library institutions',creators:[{name:'Someone Else',role:'author'}],labels:{...base.labels,subject:['library-archives']}});
catalog.coverage.rows=catalog.coverage.shown=catalog.works.length;
validatePreviewCatalog(catalog);const index=buildIndex(catalog),catalogJSON=JSON.stringify(catalog),catalogHash=hash(catalogJSON),snapshot='fixture-v1';
const titleOrder=catalog.works.map((_,i)=>i).sort((a,b)=>catalog.works[a].title.localeCompare(catalog.works[b].title,'en')||catalog.works[a].id.localeCompare(catalog.works[b].id,'en'));
const overlayJSON=JSON.stringify({schema:'openbibliographies.author-aggregation.v1',snapshot,catalog:'/data/catalog/'+catalogHash.slice(0,16)+'.json',catalogSha256:catalogHash,version:'fixture-'+hash(''),mappingSha256:hash(''),groups:[]});
const queries=['q=history histories','q=technology technological','q=history computing','q=computing computers','q=ΔΗΜΟΣ','q=δῆμος','','q=history of computing','q=Barbieri-Low','q=Anthony J. Barbieri-Low','q=T’ung-tsu Ch’ü','q=Ch’ü','q=O’Connor','q=晚清','q=published in 2000','q=1930s','period=qing,modern','subject=technology','discipline=gender-studies','region=china-all','author=alice-example','sort=title','sort=year-new','sort=year-old','q=computing ','q=the','q=zzzzunknown','format=book&workType=study','q=computing&limit=1&offset=1'];
for(const q of ['Anthony J. Barbieri-Low 图书馆学 档案学','Anthony J. Barbieri-Low 图书馆学与档案学','图书馆学 档案学','图书馆学与档案学'])queries.push(new URLSearchParams({q}).toString());for(const q of ['图书馆学 档案学','图书馆学与档案学'])queries.push(new URLSearchParams({q,author:'anthony-j-barbieri-low'}).toString());
const cases=queries.map(q=>{const p=new URLSearchParams(q);p.set('facets','period,format,subject');return{params:Object.fromEntries(p),result:executePublicQuery(index,parsePublicQuery(p),snapshot,true)}});
process.stdout.write(JSON.stringify({catalogJSON,retrieval:{schema:'openbibliographies.retrieval.v1',catalogSHA256:catalogHash,authorVersion:'fixture-'+hash(''),semanticsSHA256:'REPLACE_FROM_REAL_EXPORT',authorOverlayJSON:overlayJSON,authorOverlaySHA256:hash(overlayJSON),titleOrder},cases}));
