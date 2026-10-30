package mockllm

// A representative API-backed artifact for deterministic Pi integration
// tests. This fixture exercises direct fetch and the approval footprint; it is
// deliberately not evidence about the choices a live model will make.
const arxivTool = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Paper trail</title><style>
:root{color-scheme:light dark;--bg:#f5f3ee;--panel:#fff;--ink:#252d38;--muted:#69717c;--line:#dcdedc;--accent:#23559e;--on-accent:#fff}
@media(prefers-color-scheme:dark){:root{--bg:#181c22;--panel:#222830;--ink:#e8eaed;--muted:#a9b2be;--line:#3b4450;--accent:#9ebffc;--on-accent:#14233c}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font:16px/1.6 system-ui,sans-serif}main{max-width:850px;margin:48px auto;padding:0 24px}h1{margin:0;font-size:2.5rem;letter-spacing:-.04em}.lede{color:var(--muted);margin:4px 0 28px}form{display:flex;gap:10px;flex-wrap:wrap}label{width:100%;font-size:13px;font-weight:600}input{min-width:0;flex:1;padding:12px;border:1px solid var(--line);border-radius:8px;background:var(--panel);color:var(--ink);font:inherit}button{border:0;border-radius:8px;background:var(--accent);color:var(--on-accent);font:600 14px system-ui;padding:0 20px}button:disabled{opacity:.6}#status{color:var(--muted);min-height:26px;margin:18px 0}article{background:var(--panel);border:1px solid var(--line);border-radius:12px;padding:22px;margin:14px 0}h2{font-size:19px;line-height:1.35;margin:0 0 6px}a{color:var(--accent)}.authors,time{color:var(--muted);font-size:13px}article p{margin:14px 0 0}
</style></head><body><main><h1>Paper trail</h1><p class="lede">A quiet place to follow the latest research on arXiv.</p>
<form id="search"><label for="query">Search papers</label><input id="query" value="graph neural networks" required><button id="submit" type="submit">Find papers</button></form>
<p id="status" role="status">Search to load papers. Exhibit will ask before the API can connect.</p><section id="results" aria-label="Papers"></section>
</main><script>
const queryInput=document.getElementById('query');
queryInput.value=localStorage.getItem('arxiv-query')||queryInput.value;
document.getElementById('search').addEventListener('submit',async function(event){
 event.preventDefault();const query=queryInput.value.trim();if(!query)return;
 const status=document.getElementById('status');const button=document.getElementById('submit');
 button.disabled=true;status.textContent='Finding papers…';
 try{
  const response=await fetch('https://export.arxiv.org/api/query?search_query='+encodeURIComponent('all:'+query)+'&max_results=12&sortBy=submittedDate&sortOrder=descending');
  if(!response.ok)throw new Error('The API returned '+response.status);
  const feed=new DOMParser().parseFromString(await response.text(),'application/xml');
  if(feed.querySelector('parsererror'))throw new Error('Could not read the paper feed');
  const entries=Array.from(feed.querySelectorAll('entry'));const results=document.getElementById('results');results.replaceChildren();
  entries.forEach(function(entry){
   const article=document.createElement('article');const title=document.createElement('h2');const link=document.createElement('a');
   const paperURL=entry.querySelector('id').textContent;const parsedURL=new URL(paperURL);
   link.href=parsedURL.protocol==='http:'?'https://'+parsedURL.host+parsedURL.pathname:paperURL;
   link.textContent=entry.querySelector('title').textContent.replace(/\s+/g,' ').trim();title.appendChild(link);article.appendChild(title);
   const authors=document.createElement('div');authors.className='authors';authors.textContent=Array.from(entry.querySelectorAll('author name')).map(function(name){return name.textContent}).join(', ');article.appendChild(authors);
   const summary=document.createElement('p');summary.textContent=entry.querySelector('summary').textContent.replace(/\s+/g,' ').trim();article.appendChild(summary);results.appendChild(article);
  });
  localStorage.setItem('arxiv-query',query);status.textContent=entries.length?entries.length+' papers · newest first':'No papers found. Try a broader topic.';
 }catch(error){status.textContent='Could not load papers. Check network and relay approval, then try again.';}
 finally{button.disabled=false;}
});
</script></body></html>`
