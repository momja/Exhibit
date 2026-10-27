/* Source/widget conflicts must preserve the visible editor and its read token.
 * The successful retry uses the returned token, never a newer head's token. */
import {test} from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {loadPageScript, recordingApi} from './testdom.mjs';
const script = path.join(path.dirname(fileURLToPath(import.meta.url)), '../../internal/api/assets/gallery/edit.js');

async function load(responses, widgetBody = '<b>my tile</b>') {
 const api = recordingApi(responses);
 const timers = [];
 const page = loadPageScript([], {
  TOKEN:'', READ_ONLY:false, ID:'art-1', INITIAL_SOURCE_FOOTPRINT:[],
  allowlist:[], unapproved:[], blocked:[],
  downloadsApproved:false, clipboardApproved:false, linksApproved:false,
  cameraApproved:false, microphoneApproved:false, geolocationApproved:false,
  apiFetch:api.apiFetch, confirm:()=>true,
  CustomEvent:class {constructor(type) {this.type=type;}},
  // Navigation must wait until both documents have been saved.
  setTimeout:callback=>timers.push(callback)
 }, {
  title:{value:'Draft title'},
  body:{value:'<p>my source</p>', dataset:{revision:'source-v1'}},
  'widget-src':{value:widgetBody, dataset:{revision:'widget-v1'}},
  'scan-result':{style:{}}
 });
 const module = await page.loadModule(script, ['save', 'artifactDirty', 'saveEditorChanges']);
 Object.assign(page.context, module);
 return {...page,api,timers};
}
const conflict = {ok:false,status:409,json:async()=>({error:'changed elsewhere'})};
const sourceSaved = {ok:true,status:200,json:async()=>({artifact:{source_revision:'source-v2'}})};
const widgetSaved = {ok:true,status:200,json:async()=>({widget_revision:'widget-v2'})};

test('the page Save persists both documents and stays in the editor', async()=>{
 let finishWidget;
 const pendingWidget = new Promise(resolve=>{finishWidget=resolve;});
 const p=await load([sourceSaved,pendingWidget]);
 let refreshed=0;
 p.document.body.addEventListener('exhibit:widget-saved',()=>refreshed++);
 const saving=p.context.save();
 await new Promise(resolve=>setImmediate(resolve));
 assert.equal(p.api.calls.length,2);
 assert.equal(p.api.calls[0].method,'PATCH');
 assert.equal(p.api.calls[1].method,'PUT');
 assert.equal(p.api.calls[1].path,'/api/artifacts/art-1/widget');
 assert.equal(JSON.parse(p.api.calls[1].body).body,'<b>my tile</b>');
 assert.equal(p.timers.length,0);
 assert.equal(p.byId('status').textContent,'Saving…');
 finishWidget(widgetSaved);
 await saving;
 assert.equal(refreshed,1);
 assert.equal(p.byId('status').textContent,'✓ Saved');
 assert.equal(p.byId('widget-src').dataset.revision,'widget-v2');
 assert.equal(p.byId('widget-summary-text').textContent,'custom tile');
 assert.equal(p.timers.length,0);
 assert.equal(p.window.location.href,undefined);
});

test('an empty default widget is left absent', async()=>{
 const p=await load([sourceSaved],' \n ');
 await p.context.save();
 assert.equal(p.api.calls.length,1);
 assert.equal(p.byId('status').textContent,'✓ Saved');
 assert.equal(p.timers.length,0);
});

test('Save and leave accepts whitespace typed into an empty default widget', async()=>{
 const p=await load([sourceSaved],'');
 p.byId('widget-src').value=' \n ';
 assert.equal(p.context.artifactDirty(),true);
 assert.equal(await p.context.saveEditorChanges(),true);
 assert.equal(p.context.artifactDirty(),false);
 assert.deepEqual(p.api.calls.map(call=>call.method),['PATCH']);
 assert.equal(p.byId('status').textContent,'✓ Saved');
});

test('the page Save creates a newly written widget', async()=>{
 const p=await load([sourceSaved,widgetSaved],'');
 p.byId('widget-src').value='<b>new tile</b>';
 await p.context.save();
 assert.equal(JSON.parse(p.api.calls[1].body).body,'<b>new tile</b>');
 assert.equal(p.byId('widget-generate-label').textContent,'Regenerate');
});

test('a source conflict preserves edits and sends no follow-up requests', async()=>{
 const p=await load([conflict]);
 await p.context.save();
 assert.equal(p.api.calls.length,1);
 assert.equal(JSON.parse(p.api.calls[0].body).expected_source_revision,'source-v1');
 assert.equal(p.byId('body').value,'<p>my source</p>');
 assert.equal(p.byId('body').dataset.revision,'source-v1');
 assert.equal(p.byId('title').value,'Draft title');
 assert.match(p.byId('status').textContent,/Copy them before reloading/);
});

test('success remembers the source token returned for the submitted bytes', async()=>{
 const p=await load([sourceSaved,widgetSaved,conflict]);
 await p.context.save();
 p.byId('body').value='<p>another edit</p>';
 await p.context.save();
 assert.equal(JSON.parse(p.api.calls[2].body).expected_source_revision,'source-v2');
 assert.equal(p.byId('body').value,'<p>another edit</p>');
});

test('a widget conflict preserves its buffer and read token', async()=>{
 const p=await load([sourceSaved,conflict]);
 await p.context.save();
 assert.equal(p.api.calls.length,2);
 assert.equal(JSON.parse(p.api.calls[1].body).expected_widget_revision,'widget-v1');
 assert.equal(p.byId('widget-src').value,'<b>my tile</b>');
 assert.equal(p.byId('widget-src').dataset.revision,'widget-v1');
 assert.match(p.byId('widget-status').textContent,/Copy them before reloading/);
 assert.match(p.byId('status').textContent,/Artifact saved, but the widget was not saved/);
 assert.match(p.byId('status').textContent,/Copy them before reloading/);
 assert.equal(p.timers.length,0);
});

test('a second widget save uses the first save response token', async()=>{
 const p=await load([sourceSaved,widgetSaved,sourceSaved,conflict]);
 await p.context.save();
 await p.context.save();
 assert.equal(JSON.parse(p.api.calls[3].body).expected_widget_revision,'widget-v2');
});

test('a failed widget save preserves edits and does not navigate', async()=>{
 const p=await load([sourceSaved,{ok:false,status:500,text:async()=>'Storage unavailable'}]);
 await p.context.save();
 assert.match(p.byId('status').textContent,/Storage unavailable/);
 assert.equal(p.byId('widget-src').value,'<b>my tile</b>');
 assert.equal(p.byId('widget-src').dataset.revision,'widget-v1');
 assert.equal(p.timers.length,0);
});

test('retrying a failed widget save retains the source network approval gate', async()=>{
 const sourceWithNewOrigin={ok:true,status:200,json:async()=>({
  artifact:{source_revision:'source-v2'},
  footprint_changed:true,network_footprint:['https://new.example.com']
 })};
 const p=await load([sourceWithNewOrigin,conflict,sourceSaved,widgetSaved]);
 await p.context.save();
 assert.equal(p.timers.length,0);
 await p.context.save();
 assert.equal(p.byId('scan-result').style.display,'block');
 assert.match(p.byId('scan-result').innerHTML,/https:\/\/new\.example\.com/);
 assert.equal(p.timers.length,0);
});

test('clearing an existing widget requires explicit removal', async()=>{
 const p=await load([sourceSaved]);
 p.byId('widget-src').value='';
 await p.context.save();
 assert.equal(p.api.calls.length,1);
 assert.match(p.byId('status').textContent,/Use Remove/);
 assert.equal(p.timers.length,0);
});

test('Save after removing a widget keeps the default tile', async()=>{
 const p=await load([{ok:true,status:204,headers:new Map([['X-Widget-Revision','widget-removed']])},sourceSaved]);
 await p.byId('widget-remove').click();
 await p.context.save();
 assert.deepEqual(p.api.calls.map(call=>call.method),['DELETE','PATCH']);
 assert.equal(p.byId('widget-summary-text').textContent,'default tile');
 assert.equal(p.byId('status').textContent,'✓ Saved');
});

test('a conflicting widget removal leaves the editor intact', async()=>{
 const p=await load([conflict]);
 await p.byId('widget-remove').click();
 assert.equal(p.api.calls[0].method,'DELETE');
 assert.equal(p.byId('widget-src').value,'<b>my tile</b>');
 assert.equal(p.byId('widget-src').dataset.revision,'widget-v1');
 assert.match(p.byId('widget-status').textContent,/Copy them before reloading/);
});

test('typing while a source save is pending keeps the later draft dirty', async()=>{
 let finishSource;
 const response = new Promise(resolve=>{finishSource=resolve;});
 const p=await load([response,widgetSaved]);
 const saving=p.context.save();
 p.byId('body').value='<p>typed while saving</p>';
 p.byId('title').value='A later title';
 finishSource(sourceSaved);
 await saving;
 assert.equal(p.byId('body').dataset.revision,'source-v2');
 assert.equal(p.byId('body').value,'<p>typed while saving</p>');
 assert.equal(p.byId('title').value,'A later title');
 assert.equal(p.context.artifactDirty(),true);
});

test('a successful save clears dirtiness without redirecting', async()=>{
 const p=await load([sourceSaved,widgetSaved]);
 p.byId('body').value='<p>new source</p>';
 assert.equal(p.context.artifactDirty(),true);
 assert.equal(await p.context.save(),true);
 assert.equal(p.context.artifactDirty(),false);
 assert.equal(p.window.location.href,undefined);
});

test('a source conflict leaves unsaved edits dirty', async()=>{
 const p=await load([conflict]);
 p.byId('body').value='<p>unsaved draft</p>';
 assert.equal(await p.context.save(),false);
 assert.equal(p.context.artifactDirty(),true);
});

test('Save and leave joins a pending save and refuses to leave with later edits', async()=>{
 let finishSource;
 const response = new Promise(resolve=>{finishSource=resolve;});
 const p=await load([response,widgetSaved]);
 p.byId('title').value='Submitted title';
 const first=p.context.saveEditorChanges();
 await new Promise(resolve=>setImmediate(resolve));
 const second=p.context.saveEditorChanges();
 assert.equal(first,second);
 assert.equal(p.api.calls.length,1);
 p.byId('body').value='<p>typed after Save</p>';
 finishSource(sourceSaved);
 assert.equal(await first,false);
 assert.equal(p.context.artifactDirty(),true);
 assert.equal(p.byId('body').value,'<p>typed after Save</p>');
});
