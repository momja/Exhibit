/* Source/widget conflicts must preserve the visible editor and its read token.
 * The successful retry uses the returned token, never a newer head's token. */
import {test} from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {loadPageScript, recordingApi} from './testdom.mjs';
const script = path.join(path.dirname(fileURLToPath(import.meta.url)), '../../internal/api/assets/gallery/edit.js');

function load(responses) {
 const api = recordingApi(responses);
 const page = loadPageScript(script, {
  TOKEN:'', READ_ONLY:false, ID:'art-1',
  allowlist:[], unapproved:[], blocked:[],
  downloadsApproved:false, clipboardApproved:false, linksApproved:false,
  cameraApproved:false, microphoneApproved:false, geolocationApproved:false,
  apiFetch:api.apiFetch, confirm:()=>true,
  CustomEvent:class {constructor(type) {this.type=type;}},
  // Keep a successful source save from navigating this test's fake page.
  setTimeout:()=>0
 }, {
  title:{value:'Draft title'},
  body:{value:'<p>my source</p>', dataset:{revision:'source-v1'}},
  'widget-src':{value:'<b>my tile</b>', dataset:{revision:'widget-v1'}},
  'scan-result':{style:{}}
 });
 return {...page,api};
}
const conflict = {ok:false,status:409,json:async()=>({error:'changed elsewhere'})};

test('a source conflict preserves edits and sends no follow-up requests', async()=>{
 const p=load([conflict]);
 await p.context.save();
 assert.equal(p.api.calls.length,1);
 assert.equal(JSON.parse(p.api.calls[0].body).expected_source_revision,'source-v1');
 assert.equal(p.byId('body').value,'<p>my source</p>');
 assert.equal(p.byId('body').dataset.revision,'source-v1');
 assert.equal(p.byId('title').value,'Draft title');
 assert.match(p.byId('status').textContent,/Copy them before reloading/);
});

test('success remembers the source token returned for the submitted bytes', async()=>{
 const p=load([{ok:true,status:200,json:async()=>({artifact:{source_revision:'source-v2'}})},conflict]);
 await p.context.save();
 p.byId('body').value='<p>another edit</p>';
 await p.context.save();
 assert.equal(JSON.parse(p.api.calls[1].body).expected_source_revision,'source-v2');
 assert.equal(p.byId('body').value,'<p>another edit</p>');
});

test('a widget conflict preserves its buffer and read token', async()=>{
 const p=load([conflict]);
 await p.byId('widget-save').click();
 assert.equal(p.api.calls.length,1);
 assert.equal(JSON.parse(p.api.calls[0].body).expected_widget_revision,'widget-v1');
 assert.equal(p.byId('widget-src').value,'<b>my tile</b>');
 assert.equal(p.byId('widget-src').dataset.revision,'widget-v1');
 assert.match(p.byId('widget-status').textContent,/Copy them before reloading/);
});

test('a second widget save uses the first save response token', async()=>{
 const p=load([{ok:true,status:200,json:async()=>({widget_revision:'widget-v2'})},conflict]);
 await p.byId('widget-save').click();
 await p.byId('widget-save').click();
 assert.equal(JSON.parse(p.api.calls[1].body).expected_widget_revision,'widget-v2');
});

test('a conflicting widget removal leaves the editor intact', async()=>{
 const p=load([conflict]);
 await p.byId('widget-remove').click();
 assert.equal(p.api.calls[0].method,'DELETE');
 assert.equal(p.byId('widget-src').value,'<b>my tile</b>');
 assert.equal(p.byId('widget-src').dataset.revision,'widget-v1');
 assert.match(p.byId('widget-status').textContent,/Copy them before reloading/);
});
