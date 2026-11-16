import { test, expect } from '../fixtures';
import { renderOrigin } from '../environment';
import { mkdir, writeFile } from 'node:fs/promises';

const evidence = '/tmp/exhibit-review/banner';
const body = `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><style>
html,body{margin:0;overflow:hidden;font:16px system-ui;color-scheme:light dark}main{height:100vh;display:grid;grid-template-rows:48px 1fr 48px;background:light-dark(#eef2f8,#151d2a);color:light-dark(#24324a,#e1e8f4)}h1{font-size:18px;margin:12px}canvas{width:100%;height:100%;min-height:0;background:light-dark(#d9e5f5,#283c58)}footer{position:fixed;bottom:0;height:48px;left:0;right:0;display:flex;align-items:center;justify-content:center;background:light-dark(#fff,#182333)}button{font:inherit;padding:6px 18px}</style></head><body><main><h1>Viewport canvas</h1><canvas aria-label="Drawing surface" onpointerdown="this.dataset.drawn='yes';this.getContext('2d').fillRect(0,0,20,20)"></canvas><footer><button onclick="this.textContent='Saved'">Save drawing</button></footer></main></body></html>`;

test('attribution viewport, theme, keyboard, hostile code and embed behavior', async ({page,api,artifacts}, info) => {
 test.setTimeout(120_000);
 page.on('console',msg=>console.log('browser:',msg.type(),msg.text()));
 page.on('requestfailed',req=>console.log('request failed:',req.url(),req.failure()));
 await mkdir(evidence,{recursive:true});
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
  expect(csp).not.toContain('allow-popups'); expect(csp).not.toContain('allow-top-navigation');
  const tool=page.frameLocator('iframe');
  await expect(tool.getByRole('button')).toBeVisible();
  // WebKit can restore focus inside the tool on reload. The bar must never
  // acquire it automatically; explicitly start a keyboard traversal at the link.
  await expect(page.getByRole('link',{name:'made with exhibit'})).not.toBeFocused();
  await page.getByRole('link',{name:'made with exhibit'}).focus();
  await expect(page.getByRole('link',{name:'made with exhibit'})).toBeFocused();
  await page.keyboard.press('Tab');
  await expect(tool.getByRole('button')).toBeFocused();
  await page.keyboard.press('Enter');
  await tool.getByRole('button').click(); await expect(tool.getByRole('button')).toHaveText('Saved');
  await tool.locator('canvas').click();
  await expect(tool.locator('canvas')).toHaveAttribute('data-drawn','yes');
  const frame=page.frames()[1];
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
 const child=page.frames()[2];
 expect(await child.evaluate(()=>{try{parent.document.body.remove();return 'removed'}catch{return 'blocked'}})).toBe('blocked');
 await writeFile(`${evidence}/${info.project.name}-measurements.json`,JSON.stringify(measurements,null,2));
});

test('owner attribution selection persists through the server-rendered share panel',async({page,api,artifacts})=>{
 const artifact=await artifacts.create('<h1>Owner tool</h1>');
 await page.goto(`/artifacts/${artifact.id}`);
 await page.locator('#share-open').click();
 await expect(page.locator('#share-attribution')).toHaveValue('inherit');
 await page.locator('#share-attribution').selectOption('show');
 await expect(page.locator('#share-body')).toBeEnabled();
 await expect(page.locator('#share-attribution')).toHaveValue('show');
 expect((await (await api.get(`/api/artifacts/${artifact.id}`)).json()).share_attribution).toBe('show');
 await page.screenshot({path:'/tmp/exhibit-review/banner/share-panel.png'});
});

test('native framed anchors under the actual share CSP sandbox',async({page,api,artifacts},info)=>{
 test.setTimeout(120_000);
 const artifact=await artifacts.create('<p>Native anchor measurement</p>');
 await api.patch(`/api/artifacts/${artifact.id}`,{data:{share_attribution:'show'}});
 const {share}=await(await api.post('/api/shares',{data:{artifact_id:artifact.id}})).json();
 const headers=(await api.get(`${renderOrigin}/s/${share.id}`)).headers();
 const csp=headers['content-security-policy'];
 expect(csp).toContain('sandbox allow-scripts allow-forms');
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
 await expect.poll(()=>page.frames()[1].url()).toBe('https://exhibit.dizzard.net/native');
 await writeFile(`${evidence}/${info.project.name}-native-anchors.json`,JSON.stringify({csp,messages,popup:'blocked',top:'blocked',self:'navigated child',decision:'inert framed attribution avoids replacing the embedded tool'},null,2));
});
