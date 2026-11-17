import { test, expect } from '../fixtures';
import type { Page, Frame } from '@playwright/test';
import { renderOrigin } from '../environment';
import { mkdir, writeFile } from 'node:fs/promises';

const evidence = process.env.EXHIBIT_ATTRIBUTION_EVIDENCE || '/tmp/exhibit-review/banner/r2';
// Resolve the direct child through its owning iframe, never global frame order.
async function childFrame(parent: Page | Frame): Promise<Frame> {
 const element = await parent.locator('iframe[title]').elementHandle();
 const frame = await element!.contentFrame();
 expect(frame).not.toBeNull();
 return frame!;
}
test.beforeEach(async()=>{await mkdir(evidence,{recursive:true})});
const body = `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><style>
html,body{margin:0;overflow:hidden;font:16px system-ui;color-scheme:light dark}main{height:100vh;display:grid;grid-template-rows:48px 1fr 48px;background:light-dark(#eef2f8,#151d2a);color:light-dark(#24324a,#e1e8f4)}h1{font-size:18px;margin:12px}canvas{width:100%;height:100%;min-height:0;background:light-dark(#d9e5f5,#283c58)}footer{position:fixed;bottom:0;height:48px;left:0;right:0;display:flex;align-items:center;justify-content:center;background:light-dark(#fff,#182333)}button{font:inherit;padding:6px 18px}</style></head><body><main><h1>Viewport canvas</h1><canvas aria-label="Drawing surface" onpointerdown="this.dataset.drawn='yes';this.getContext('2d').fillRect(0,0,20,20)"></canvas><footer><button onclick="this.textContent='Saved'">Save drawing</button></footer></main></body></html>`;

test('attribution viewport, theme, keyboard, hostile code and embed behavior', async ({page,api,artifacts}, info) => {
 test.setTimeout(120_000);
 page.on('console',msg=>console.log('browser:',msg.type(),msg.text()));
 page.on('requestfailed',req=>console.log('request failed:',req.url(),req.failure()));
 const artifact = await artifacts.create(body,'Viewport canvas');
 expect((await api.patch(`/api/artifacts/${artifact.id}`,{data:{share_attribution:'show'}})).ok()).toBeTruthy();
 const response=await api.post('/api/shares',{data:{artifact_id:artifact.id}});
 expect(response.ok()).toBeTruthy();
 const {share}=await response.json();
 const url=`${renderOrigin}/s/${share.id}`;
 const measurements:any[]=[];
 for(const width of [320,1280]) for(const colorScheme of ['light','dark'] as const){
  await page.setViewportSize({width,height:720}); await page.emulateMedia({colorScheme});
  const response=await page.goto(url); const csp=response!.headers()['content-security-policy'];
  expect(csp).toContain('sandbox allow-scripts allow-forms');
 expect(csp).not.toContain('frame-src');
  expect(csp).not.toContain('allow-popups'); expect(csp).not.toContain('allow-top-navigation');
  const tool=page.frameLocator('iframe');
  await expect(tool.getByRole('button')).toBeVisible();
  // WebKit can restore focus inside the tool on reload. The bar must never
  // acquire it automatically; explicitly start a keyboard traversal at the link.
  await expect(page.getByRole('link',{name:'made with exhibit'})).not.toBeFocused();
  await page.getByRole('link',{name:'made with exhibit'}).focus();
  await expect(page.getByRole('link',{name:'made with exhibit'})).toBeFocused();
  await page.screenshot({path:`${evidence}/${info.project.name}-focus-${width}-${colorScheme}.png`});
  await page.keyboard.press('Tab');
  await expect(tool.getByRole('button')).toBeFocused();
  await page.keyboard.press('Enter');
  await tool.getByRole('button').click(); await expect(tool.getByRole('button')).toHaveText('Saved');
  await tool.locator('canvas').click();
  await expect(tool.locator('canvas')).toHaveAttribute('data-drawn','yes');
  const frame=(await childFrame(page));
  const geometry=await frame.evaluate(()=>({height:innerHeight,tool:document.querySelector('main')!.getBoundingClientRect().height,bottom:document.querySelector('footer')!.getBoundingClientRect().bottom,overflow:document.documentElement.scrollHeight>innerHeight}));
  expect(geometry).toEqual({height:684,tool:684,bottom:684,overflow:false});
  await page.keyboard.press('Tab'); // no focus trap; ordinary document/frame tab order
  await page.screenshot({path:`${evidence}/${info.project.name}-${width}-${colorScheme}.png`});
  measurements.push({width,colorScheme,csp,geometry});
 }
 // No network request to the product is needed to measure navigation.
 await page.route('https://exhibit.dizzard.net/**',route=>route.fulfill({contentType:'text/html',body:'<title>Product destination</title>'}));
 await page.getByRole('link',{name:'made with exhibit'}).click();
 await expect(page).toHaveURL('https://exhibit.dizzard.net/');
 await page.goto('about:blank');
 await page.goto('http://127.0.0.1:18382/?src='+encodeURIComponent(url));
 const embedURL=page.url();
 const shareFrame=page.frameLocator('iframe');
 const inert=shareFrame.locator('#exhibit-attribution');
 await expect(inert).not.toHaveAttribute('href');
 await inert.click(); expect(page.url()).toBe(embedURL);
 await expect(shareFrame.frameLocator('iframe').getByRole('button')).toBeVisible();
 await page.screenshot({path:`${evidence}/${info.project.name}-third-party.png`});
 measurements.push({framedLink:'inert; no href; click leaves parent URL unchanged',topLevelLink:'self navigation succeeded with actual sandbox, no popup/top-navigation tokens'});
 // Child cannot remove the bar or overlay parent chrome.
 const child=(await childFrame(await childFrame(page)));
 expect(await child.evaluate(()=>{try{parent.document.body.remove();return 'removed'}catch{return 'blocked'}})).toBe('blocked');
 await writeFile(`${evidence}/${info.project.name}-measurements.json`,JSON.stringify(measurements,null,2));
});

test('owner attribution selection persists through the server-rendered share panel',async({page,api,artifacts},info)=>{
 const artifact=await artifacts.create('<h1>Owner tool</h1>');
 await page.goto(`/artifacts/${artifact.id}`);
 await page.locator('#share-open').click();
 await expect(page.locator('#share-attribution')).toHaveValue('inherit');
 await page.screenshot({path:`${evidence}/${info.project.name}-panel-inherit.png`});
 await page.locator('#share-attribution').selectOption('show');
 await expect(page.locator('#share-body')).toBeEnabled();
 await expect(page.locator('#share-attribution')).toHaveValue('show');
 expect((await (await api.get(`/api/artifacts/${artifact.id}`)).json()).share_attribution).toBe('show');
 await expect(page.locator('#share-attribution-help')).toContainText('Only affects the public /s/ link');
 await expect(page.locator('#share-attribution-help')).toContainText('plain attribution text');
 await page.screenshot({path:`${evidence}/${info.project.name}-panel-show.png`});
 await page.locator('#share-attribution').selectOption('hide');
 await expect.poll(async()=>(await(await api.get(`/api/artifacts/${artifact.id}`)).json()).share_attribution).toBe('hide');
 await expect(page.locator('#share-attribution')).toHaveValue('hide');
 await page.screenshot({path:`${evidence}/${info.project.name}-panel-hide.png`});
});

test('native framed anchors under the actual share CSP sandbox',async({page,api,artifacts},info)=>{
 test.setTimeout(120_000);
 const artifact=await artifacts.create('<p>Native anchor measurement</p>');
 await api.patch(`/api/artifacts/${artifact.id}`,{data:{share_attribution:'show'}});
 const {share}=await(await api.post('/api/shares',{data:{artifact_id:artifact.id}})).json();
 const headers=(await api.get(`${renderOrigin}/s/${share.id}`)).headers();
 const csp=headers['content-security-policy'];
 expect(csp).toContain('sandbox allow-scripts allow-forms');
 expect(csp).not.toContain('frame-src');
 await page.route(`${renderOrigin}/native-anchor`,r=>r.fulfill({headers:{'content-security-policy':csp,'content-type':'text/html'},body:`<a href="https://exhibit.dizzard.net/native" target="_blank">popup</a><a href="https://exhibit.dizzard.net/native" target="_top">top</a><a href="https://exhibit.dizzard.net/native">self</a>`}));
 await page.route('https://exhibit.dizzard.net/**',r=>r.fulfill({contentType:'text/html',body:'<title>Native destination</title>'}));
 const messages:string[]=[];page.on('console',msg=>messages.push(msg.text()));
 await page.goto('http://127.0.0.1:18382/?src='+encodeURIComponent(renderOrigin+'/native-anchor'));
 const embedURL=page.url();
 const frame=page.frameLocator('iframe');
 await frame.getByText('popup',{exact:true}).click();
 expect(page.context().pages()).toHaveLength(1);
 await frame.getByText('top',{exact:true}).click();
 expect(page.url()).toBe(embedURL);
 await frame.getByText('self',{exact:true}).click();
 await expect.poll(async()=>(await childFrame(page)).url()).toBe('https://exhibit.dizzard.net/native');
 await writeFile(`${evidence}/${info.project.name}-native-anchors.json`,JSON.stringify({csp,messages,popup:'blocked',top:'blocked',self:'navigated child',decision:'inert framed attribution avoids replacing the embedded tool'},null,2));
});

test('initial keyboard input, autofocus and accessible return to attribution',async({page,api,artifacts},info)=>{
 const artifact=await artifacts.create(`<!doctype html><html><head><title>Keyboard tool</title></head><body><input aria-label="First input" autofocus><button>Next</button><script>window.keys=0;window.addEventListener('keydown',()=>window.keys++);</script></body></html>`);
 const {share}=await(await api.post('/api/shares',{data:{artifact_id:artifact.id}})).json();
 const results=[];
 for(const mode of ['hide','show']){
  await api.patch(`/api/artifacts/${artifact.id}`,{data:{share_attribution:mode}});
  await page.goto('about:blank');
  await page.bringToFront();
  await page.goto(`${renderOrigin}/s/${share.id}?x=1#frag`);
  const tool=mode==='show'?(await childFrame(page)):page.mainFrame();
  if(mode==='hide'||info.project.name==='chromium')await expect(tool.locator('input'),mode).toBeFocused();
  else await expect(page.locator('iframe')).toBeFocused();
  await page.keyboard.press('a'); await page.keyboard.press('b');
  expect(await tool.evaluate(()=>(window as any).keys)).toBe(2);
  results.push({mode,autofocus:await tool.locator('input').inputValue()});
  if(mode==='hide'||info.project.name==='chromium')await expect(tool.locator('input')).toHaveValue('ab');
  else expect(['','ab']).toContain(await tool.locator('input').inputValue()); // WebKit suppresses cross-origin form autofocus.
  if(mode==='show'){
   // WebKit may leave initial focus on the frame body. Reach the first
   // field with a real Tab before testing reverse traversal to the banner.
   if(!await tool.locator('input').evaluate(el=>el===document.activeElement))await page.keyboard.press('Tab');
   await expect(tool.locator('input')).toBeFocused();
   // Reverse traversal reaches the parent link, then forward returns to the tool.
   await page.keyboard.press('Shift+Tab');
   await expect(page.locator('#exhibit-attribution')).toBeFocused();
   await page.keyboard.press('Tab');
   await expect(tool.locator('input')).toBeFocused();
   await page.keyboard.press('Shift+Tab');
   await tool.goto('about:blank');
   await expect(page.locator('#exhibit-attribution')).toBeFocused();
  }
 }
 await writeFile(`${evidence}/${info.project.name}-initial-focus.json`,JSON.stringify(results,null,2));
});

test('scroll-heavy tool and light narrow/short embeds remain usable',async({page,api,artifacts},info)=>{
 const artifact=await artifacts.create(`<!doctype html><meta name="viewport" content="width=1024"><style>body{margin:0;font:16px system-ui}section{height:400px;padding:16px;background:#eef2f8;border-bottom:1px solid #888}@media print{section{break-after:page}}</style>${Array.from({length:10},(_,i)=>`<section>Section ${i+1}</section>`).join('')}<button onclick="this.textContent='Done'">Finish document</button>`,'Long document');
 await api.patch(`/api/artifacts/${artifact.id}`,{data:{share_attribution:'show'}});
 const {share}=await(await api.post('/api/shares',{data:{artifact_id:artifact.id}})).json();
 const url=`${renderOrigin}/s/${share.id}?x=1#frag`;
 await page.setViewportSize({width:320,height:720}); await page.emulateMedia({colorScheme:'light'});
 await page.goto(url);
 const tool=(await childFrame(page));
 await tool.getByRole('button').scrollIntoViewIfNeeded();
 await tool.getByRole('button').click(); await expect(tool.getByRole('button')).toHaveText('Done');
 expect(await tool.evaluate(()=>scrollY)).toBeGreaterThan(3000);
 await page.screenshot({path:`${evidence}/${info.project.name}-scroll-bottom.png`});
 const costs=await tool.evaluate(()=>({location:location.href,search:location.search,hash:location.hash,top:self===top,width:innerWidth,height:innerHeight,scrollHeight:document.documentElement.scrollHeight}));
 expect(costs).toMatchObject({location:'about:srcdoc',search:'',hash:'',top:false,width:320,height:684});
 await writeFile(`${evidence}/${info.project.name}-wrapper-costs.json`,JSON.stringify(costs,null,2));
 await tool.evaluate(()=>sessionStorage.setItem('draft','temporary'));
 expect(await tool.evaluate(()=>sessionStorage.getItem('draft'))).toBe('temporary');
 await page.reload();
 expect(await (await childFrame(page)).evaluate(()=>sessionStorage.getItem('draft'))).toBeNull();
 if(info.project.name==='chromium'){
  const counts=[];
  for(const mode of ['hide','show']){
   await api.patch(`/api/artifacts/${artifact.id}`,{data:{share_attribution:mode}});
   await page.goto('about:blank');
   await page.goto(url);
   const pdf=await page.pdf({path:`${evidence}/chromium-long-${mode}.pdf`});
   counts.push({mode,pages:(pdf.toString('latin1').match(/\/Type\s*\/Page\b/g)||[]).length});
  }
  expect(counts[0].pages).toBeGreaterThan(counts[1].pages);
  await writeFile(`${evidence}/chromium-print.json`,JSON.stringify(counts,null,2));
 }
 await api.patch(`/api/artifacts/${artifact.id}`,{data:{share_attribution:'show'}});
 for(const height of [600,144]){
  await page.goto('http://127.0.0.1:18382/?src='+encodeURIComponent(url));
  const outer=page.locator('iframe'); await outer.evaluate((el,h)=>el.style.height=`${h}px`,height);
  const shareFrame=(await childFrame(page));
  await expect(shareFrame.locator('#exhibit-attribution')).not.toHaveAttribute('href');
  await shareFrame.locator('#exhibit-attribution').evaluate(el=>(el as HTMLElement).focus());
  await expect(shareFrame.locator('#exhibit-attribution')).not.toBeFocused();
  const child=(await childFrame(await childFrame(page)));
  expect(await child.evaluate(()=>innerHeight)).toBe(height-36);
  await child.getByRole('button').click(); await expect(child.getByRole('button')).toHaveText('Done');
  await page.screenshot({path:`${evidence}/${info.project.name}-embed-light-320-${height}.png`,fullPage:true});
 }
});

test('native fullscreen succeeds and ungranted clipboard follows engine policy without a bridge',async({page,api,artifacts},info)=>{
 const artifact=await artifacts.create(`<button id="full" onclick="document.documentElement.requestFullscreen().then(()=>this.dataset.result='ok',e=>this.dataset.result=e.name+': '+e.message)">Fullscreen</button><button id="clip" onclick="navigator.clipboard.writeText('Attribution test').then(()=>this.dataset.result='ok',e=>this.dataset.result=e.name+': '+e.message)">Copy</button><script>window.messages=[];addEventListener('message',e=>window.messages.push(e.data));</script>`);
 const {share}=await(await api.post('/api/shares',{data:{artifact_id:artifact.id}})).json();
 const results=[];
 for(const mode of ['hide','show']){
  await api.patch(`/api/artifacts/${artifact.id}`,{data:{share_attribution:mode}});
  await page.goto(`${renderOrigin}/s/${share.id}`);
  const tool=mode==='show'?(await childFrame(page)):page.mainFrame();
  if(mode==='show'){
   await expect(page.locator('iframe')).toHaveAttribute('allow','fullscreen *; clipboard-write *');
   await expect(page.locator('iframe')).toHaveAttribute('sandbox','allow-scripts allow-forms');
   // The shim must preserve the native API, not replace it with a host request.
   expect(await tool.evaluate(()=>String(navigator.clipboard.writeText))).toContain('[native code]');
  }
  await tool.locator('#full').click();
  await expect(tool.locator('#full')).toHaveAttribute('data-result',/.+/);
  const fullscreen=await tool.locator('#full').getAttribute('data-result');
  expect(fullscreen).toBe('ok');
  await tool.evaluate(async()=>{if(document.fullscreenElement)await document.exitFullscreen()});
  await tool.locator('#clip').click();
  await expect(tool.locator('#clip')).toHaveAttribute('data-result',/.+/);
  const clipboard=await tool.locator('#clip').getAttribute('data-result');
  if(info.project.name==='webkit')expect(clipboard).toBe('ok');
  else expect(clipboard).toMatch(/^NotAllowedError:/);
  results.push({mode,fullscreen,clipboard});
 }
 await writeFile(`${evidence}/${info.project.name}-native-capabilities.json`,JSON.stringify(results,null,2));
});

test('hostless shares scroll fragments, reject devices and refuse external navigation under CSP',async({page,api,artifacts},info)=>{
 const artifact=await artifacts.create(`<!doctype html><html><head><style>body{margin:0}#space{height:2200px}#details{height:100px}</style></head><body><a href="#details">Jump to details</a><div id="space"></div><h2 id="details">Details reached</h2><a href="https://example.test/destination">External destination</a></body></html>`);
 await api.patch(`/api/artifacts/${artifact.id}`,{data:{share_attribution:'show'}});
 const {share}=await(await api.post('/api/shares',{data:{artifact_id:artifact.id}})).json();
 await page.goto(`${renderOrigin}/s/${share.id}`);
 const tool=(await childFrame(page));
 await tool.getByRole('link',{name:'Jump to details'}).click();
 expect(tool.url()).toBe('about:srcdoc');
 await expect(tool.getByRole('heading',{name:'Details reached'})).toBeInViewport();
 expect(await tool.evaluate(()=>scrollY)).toBeGreaterThan(1500);
 await page.screenshot({path:`${evidence}/${info.project.name}-fragment.png`});
 const devices=await tool.evaluate(async()=>{
  const media=navigator.mediaDevices?.getUserMedia({video:true}).then(()=> 'unexpected device access',e=>e.name)||Promise.resolve('unsupported');
  const geo=new Promise(resolve=>navigator.geolocation?navigator.geolocation.getCurrentPosition(()=>resolve('unexpected device access'),e=>resolve(`denied: ${e.code}`)):resolve('unsupported'));
  return Promise.race([Promise.all([media,geo]),new Promise(resolve=>setTimeout(()=>resolve('pending'),1500))]);
 });
 expect(devices).not.toBe('pending');
 expect(JSON.stringify(devices)).not.toContain('unexpected device access');
 const srcdoc=await page.locator('iframe').getAttribute('srcdoc');
 expect(srcdoc).not.toContain('var clipPending');
 expect(srcdoc).not.toContain('__avGeolocationResult');
 expect(srcdoc).not.toContain('__avNavigate');
 expect(srcdoc).toContain('var ANONYMOUS = true');
 const clicks:string[]=[];page.on('console',msg=>{if(msg.text().startsWith('external-default:'))clicks.push(msg.text())});
 await tool.evaluate(()=>document.addEventListener('click',event=>console.log('external-default:'+event.defaultPrevented)));
 await page.route('https://example.test/**',route=>route.fulfill({contentType:'text/html',body:'<h1>External destination reached</h1>'}));
 await tool.getByRole('link',{name:'External destination',exact:true}).click();
 await expect.poll(()=>clicks).toContain('external-default:false');
 if(info.project.name==='chromium')await expect.poll(()=>tool.url()).toBe('chrome-error://chromewebdata/');
 else {
  expect(tool.url()).toBe('about:srcdoc');
  await expect(tool.getByRole('heading',{name:'Details reached'})).toBeVisible();
 }
 await expect(page.locator('#exhibit-attribution')).toBeVisible();
 await writeFile(`${evidence}/${info.project.name}-hostless.json`,JSON.stringify({devices,fragment:'child scrolled to target without leaving srcdoc',external:'uncanceled native click refused by default-src fallback: Chromium error frame, WebKit stays in srcdoc',externalURL:tool.url()},null,2));
});


test('Chromium origin clipboard grant works directly but does not grant the opaque attributed child',async({page,context,api,artifacts},info)=>{
 test.skip(info.project.name!=='chromium','WebKit clipboard success in both modes is asserted in the native capability test; Playwright grants are Chromium-specific.');
 await context.grantPermissions(['clipboard-read','clipboard-write'],{origin:renderOrigin});
 const artifact=await artifacts.create(`<button onclick="navigator.clipboard.writeText('Attribution test').then(()=>this.dataset.result='ok',e=>this.dataset.result=e.name)">Copy</button>`);
 const {share}=await(await api.post('/api/shares',{data:{artifact_id:artifact.id}})).json();
 const results=[];
 for(const mode of ['hide','show']){
  await api.patch(`/api/artifacts/${artifact.id}`,{data:{share_attribution:mode}});
  await page.goto(`${renderOrigin}/s/${share.id}`);
  const tool=mode==='show'?(await childFrame(page)):page.mainFrame();
  await tool.getByRole('button').click();
  await expect(tool.getByRole('button')).toHaveAttribute('data-result',mode==='hide'?'ok':'NotAllowedError');
  if(mode==='hide')expect(await tool.evaluate(()=>navigator.clipboard.readText())).toBe('Attribution test');
  results.push({mode,result:await tool.getByRole('button').getAttribute('data-result')});
 }
 await writeFile(`${evidence}/chromium-granted-clipboard.json`,JSON.stringify(results,null,2));
});

test('embedded attribution preserves unfocused host and active host typing on load',async({page,api,artifacts},info)=>{
 const artifact=await artifacts.create(`<input aria-label="Tool input" autofocus><script>window.keys=0;addEventListener('keydown',()=>window.keys++);</script>`);
 await api.patch(`/api/artifacts/${artifact.id}`,{data:{share_attribution:'show'}});
 const {share}=await(await api.post('/api/shares',{data:{artifact_id:artifact.id}})).json();
 const url=`${renderOrigin}/s/${share.id}`;
 const results=[];
 for(const typing of [false,true]){
  await page.goto('http://127.0.0.1:18382/');
  // Hold the share request until the host focus state is established.
  let release!:()=>void;
  const ready=new Promise<void>(resolve=>{release=resolve});
  await page.route(url,async route=>{await ready;await route.continue()});
  await page.evaluate(({url,typing})=>{
   document.body.innerHTML='<input aria-label="Host input"><iframe title="Shared tool"></iframe>';
   if(typing)(document.querySelector('input') as HTMLInputElement).focus();
   document.querySelector('iframe')!.src=url;
  },{url,typing});
  if(typing)await page.keyboard.type('before');
  release();
  const tool=page.frameLocator('iframe').frameLocator('iframe');
  await expect(tool.getByRole('textbox')).toBeVisible();
  // Allow onload + animation-frame autofocus attempts to settle.
  await page.waitForTimeout(300);
  expect(await page.evaluate(()=>document.activeElement?.tagName)).toBe(typing?'INPUT':'BODY');
  await page.keyboard.type('after');
  if(typing)await expect(page.getByRole('textbox',{name:'Host input'})).toHaveValue('beforeafter');
  expect(await (await childFrame(await childFrame(page))).evaluate(()=>(window as any).keys)).toBe(0);
  results.push({typing,active:await page.evaluate(()=>document.activeElement?.tagName)});
  await page.screenshot({path:`${evidence}/${info.project.name}-host-focus-${typing}.png`});
  await page.unroute(url);
 }
 await writeFile(`${evidence}/${info.project.name}-host-focus.json`,JSON.stringify(results,null,2));
});


test('data download works hidden and is blocked shown by the exact sandbox',async({page,api,artifacts},info)=>{
 const artifact=await artifacts.create('<a download="hello.txt" href="data:text/plain,hello">Download file</a>');
 const {share}=await(await api.post('/api/shares',{data:{artifact_id:artifact.id}})).json();
 const results=[];
 for(const mode of ['hide','show']){
  expect((await api.patch(`/api/artifacts/${artifact.id}`,{data:{share_attribution:mode}})).ok()).toBeTruthy();
  await page.goto(`${renderOrigin}/s/${share.id}`);
  const tool=mode==='show'?await childFrame(page):page.mainFrame();
  if(mode==='show')await expect(page.locator('iframe')).toHaveAttribute('sandbox','allow-scripts allow-forms');
  // Install before clicking; bound the negative observation rather than hanging
  // on an event which the sandbox is specifically expected to suppress.
  const downloadEvent=page.waitForEvent('download',{timeout:1500}).catch(error=>{
   if(error.name==='TimeoutError')return null;
   throw error;
  });
  await tool.getByRole('link',{name:'Download file'}).click();
  const download=await downloadEvent;
  if(mode==='hide'){
   expect(download).not.toBeNull();
   expect(download!.suggestedFilename()).toBe('hello.txt');
   expect(await download!.failure()).toBeNull();
  }else expect(download).toBeNull();
  results.push({mode,download:download?.suggestedFilename()??null,observationMs:1500});
 }
 await writeFile(`${evidence}/${info.project.name}-downloads.json`,JSON.stringify(results,null,2));
});

test('confirm shows a dismissible dialog hidden but silently returns false shown',async({page,api,artifacts},info)=>{
 const artifact=await artifacts.create(`<button onclick="this.dataset.result=String(confirm('sure?'))">Confirm action</button>`);
 const {share}=await(await api.post('/api/shares',{data:{artifact_id:artifact.id}})).json();
 const results=[];
 // Dismiss from the event handler so a baseline modal cannot hang the click.
 const dialogs:{type:string,message:string}[]=[];
 const dismiss=async(dialog:any)=>{dialogs.push({type:dialog.type(),message:dialog.message()});await dialog.dismiss()};
 page.on('dialog',dismiss);
 try{
  for(const mode of ['hide','show']){
   expect((await api.patch(`/api/artifacts/${artifact.id}`,{data:{share_attribution:mode}})).ok()).toBeTruthy();
   await page.goto(`${renderOrigin}/s/${share.id}`);
   const tool=mode==='show'?await childFrame(page):page.mainFrame();
   const before=dialogs.length;
   await tool.getByRole('button').click({timeout:5000});
   await expect(tool.getByRole('button')).toHaveAttribute('data-result','false',{timeout:5000});
   // The synchronous confirm has returned: a shown dialog would already have
   // reached the listener. No arbitrary sleep or unbounded modal wait is needed.
   const observed=dialogs.slice(before);
   expect(observed).toEqual(mode==='hide'?[{type:'confirm',message:'sure?'}]:[]);
   results.push({mode,dialogs:observed,result:'false'});
  }
 }finally{page.off('dialog',dismiss)}
 await writeFile(`${evidence}/${info.project.name}-confirm.json`,JSON.stringify(results,null,2));
});
