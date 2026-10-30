/* The profile page's library kit picker (Exh-61ix) saves on change, and puts
 * the previous choice back when the save is refused. */
import {test} from 'node:test';
import assert from 'node:assert/strict';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {loadPageScript, recordingApi} from './testdom.mjs';
const script = path.join(path.dirname(fileURLToPath(import.meta.url)), '../../internal/api/assets/gallery/library-kit.js');

async function load(responses) {
 const api = recordingApi(responses);
 const page = loadPageScript([], {apiFetch: api.apiFetch}, {'library-kit': {value: ''}});
 await page.loadModule(script, []);
 return {...page, api};
}

test('choosing a kit saves it for the library', async()=>{
 const p = await load([{ok: true, status: 200, json: async()=>({kit: 'paper/v1'})}]);
 p.byId('library-kit').value = 'paper/v1';
 await p.byId('library-kit').dispatchEvent({type: 'change'});
 assert.deepEqual(p.api.calls.map(c=>[c.method, c.path, JSON.parse(c.body).kit]), [['PUT', '/api/settings/kit', 'paper/v1']]);
 assert.match(p.byId('library-kit-status').textContent, /^✓ Saved/);
 assert.equal(p.byId('library-kit').disabled, false);
});

test('a refused choice puts the saved one back', async()=>{
 const p = await load([{ok: false, status: 400, statusText: 'Bad Request', json: async()=>({error: 'kit must be a released kit ID'})}]);
 p.byId('library-kit').value = 'neon/v1';
 await p.byId('library-kit').dispatchEvent({type: 'change'});
 assert.equal(p.byId('library-kit').value, '');
 assert.equal(p.byId('library-kit-status').textContent, '✗ Not saved: kit must be a released kit ID');
});
