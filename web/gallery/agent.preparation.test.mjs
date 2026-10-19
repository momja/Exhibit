import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';
const asset = name => fileURLToPath(new URL('../../internal/api/assets/gallery/' + name, import.meta.url));
const flush = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); };
const json = body => ({ok: true, json: async () => body});
async function boot(start) {
  const calls = [];
  const streams = [];
  const timers = new Map();
  let next = 0;
  const page = loadPageScript([asset('state-api.js'), asset('network-prompt.js')], {
    TOKEN: '', READ_ONLY: false, BYOK: false, artifact: {id: 'art-1', title: 'A'},
    apiFetch: async (path, options = {}) => {
      calls.push({path, ...options});
      return path === '/api/agent/sessions' ? start() : json({});
    },
    apiEventSource: () => { const source = {close() {}}; streams.push(source); return source; },
    setTimeout: (fn, delay) => { const id = ++next; timers.set(id, {fn, delay}); return id; },
    clearTimeout: id => timers.delete(id),
    matchMedia: () => ({matches: false, addEventListener() {}}),
    CustomEvent: class { constructor(type) { this.type = type; } },
  });
  await page.loadModule(asset('agent.js'));
  await flush();
  const tick = async delay => {
    for (const [id, timer] of [...timers]) if (timer.delay === delay) { timers.delete(id); timer.fn(); }
    await flush();
  };
  const type = async text => { page.byId('input').value = text; await page.byId('input').dispatchEvent({type: 'input'}); };
  return {...page, calls, streams, tick, type};
}
const starts = page => page.calls.filter(c => c.path === '/api/agent/sessions');
const prompts = page => page.calls.filter(c => c.path.endsWith('/prompt'));
const deletes = page => page.calls.filter(c => c.method === 'DELETE');

test('typing prepares once without sending draft text or opening inference; Send claims in-flight startup', async () => {
  let resolve;
  const page = await boot(() => new Promise(r => { resolve = r; }));
  await page.type(' a b '); await page.tick(300);
  assert.equal(starts(page).length, 0);
  await page.type('draft secret'); await page.tick(300);
  await page.type('draft secret changed'); await page.tick(300);
  assert.equal(starts(page).length, 1);
  assert.deepEqual(JSON.parse(starts(page)[0].body), {artifact_id: 'art-1'});
  assert.equal(prompts(page).length, 0);
  const sending = page.window.send();
  await page.window.send();
  assert.equal(starts(page).length, 1);
  // Capture prompt identity before startup: later draft edits belong to next Send.
  await page.type('a later draft');
  resolve(json({id: 's1', sse_ticket: 't'}));
  await sending;
  assert.equal(prompts(page).length, 1);
  assert.equal(JSON.parse(prompts(page)[0].body).message, 'draft secret changed');
  assert.equal(page.byId('input').value, 'a later draft');
});

test('paste input prepares and unused preparation expires after 90 seconds', async () => {
  const page = await boot(() => json({id: 's1'}));
  await page.type('pasted words'); await page.tick(300);
  assert.equal(starts(page).length, 1);
  await page.tick(90000);
  assert.equal(deletes(page)[0].path, '/api/agent/sessions/s1');
  assert.equal(prompts(page).length, 0);
});

test('navigation disposes a late session and an interrupted Send never posts its prompt', async () => {
  let resolve;
  const page = await boot(() => new Promise(r => { resolve = r; }));
  await page.type('hello'); await page.tick(300);
  const sending = page.window.send();
  await page.dispatchOnWindow('pagehide');
  await sending;
  assert.equal(prompts(page).length, 0);
  assert.equal(page.byId('input').value, 'hello');
  resolve(json({id: 'late'}));
  await flush();
  assert.equal(deletes(page)[0].path, '/api/agent/sessions/late');
  assert.equal(deletes(page)[0].keepalive, true);
});

test('opening history discards preparation and suspends typing startup', async () => {
  const page = await boot(() => json({id: 's1'}));
  await page.type('hello'); await page.tick(300);
  page.window.openHistory(); await flush();
  assert.equal(deletes(page).length, 1);
  await page.type('another draft'); await page.tick(300);
  assert.equal(starts(page).length, 1);
});

test('a failed preparation can be retried on Send', async () => {
  let attempt = 0;
  const page = await boot(() => ++attempt === 1 ? Promise.reject(new Error('offline')) : json({id: 's2'}));
  await page.type('hello'); await page.tick(300);
  await page.window.send();
  assert.equal(starts(page).length, 2);
  assert.equal(prompts(page).length, 1);
});

test('saving a different key discards pending startup and the next Send starts fresh', async () => {
 let resolve; let starts = 0;
 const page=await boot(()=>++starts===1 ? new Promise(r=>{resolve=r;}) : json({id:'fresh'}));
 await page.type('hello');await page.tick(300);
 page.byId('key-provider').value='openai';page.byId('key-secret').value='new-key';
 await page.window.saveKey();
 resolve(json({id:'old-key'}));await flush();
 assert.equal(deletes(page)[0].path,'/api/agent/sessions/old-key');
 await page.window.send();
 assert.equal(prompts(page).length,1);assert.equal(prompts(page)[0].path,'/api/agent/sessions/fresh/prompt');
});

test('navigation during a history startup closes its late response', async () => {
 let resolve;
 const page=await boot(()=>new Promise(r=>{resolve=r;}));
 // Delegated fragment buttons use the same module-scoped resume handler.
 const button={dataset:{resume:'old-conversation',rollback:'0'}};
 const resuming=page.byId('history').click({closest:()=>button});await flush();
 await page.dispatchOnWindow('pagehide');resolve(json({id:'resumed'}));
 await resuming;await flush();
 assert.equal(deletes(page)[0].path,'/api/agent/sessions/resumed');assert.equal(prompts(page).length,0);
});


test('old stream callbacks cannot close or reconnect a replacement session', async () => {
 let count=0;const page=await boot(()=>json({id:'s'+(++count)}));
 await page.type('first');await page.window.send();
 const old=page.streams[0];
 page.byId('key-provider').value='openai';page.byId('key-secret').value='new-key';await page.window.saveKey();
 await page.type('second');await page.window.send();
 old.onmessage({data:JSON.stringify({type:'exhibit_session_closed'})});old.onerror();
 await page.tick(1000);
 await page.type('third');await page.window.send();
 assert.equal(starts(page).length,2);
 assert.equal(prompts(page).at(-1).path,'/api/agent/sessions/s2/prompt');
 assert.equal(page.streams.length,2);
});

test('Send escapes a stalled preparation, retains its draft, retries, and disposes the late session', async () => {
  let finishStalled;
  let attempts = 0;
  const page = await boot(() => ++attempts === 1
    ? new Promise(resolve => { finishStalled = resolve; }) : json({id: 'retry'}));
  await page.type('accepted draft');
  await page.tick(300);
  const sending = page.window.send();
  await flush();
  assert.equal(starts(page).length, 1, 'Send reuses the pending POST');
  await page.tick(90000);
  await sending;
  assert.equal(prompts(page).length, 0);
  assert.equal(page.byId('input').value, 'accepted draft');
  assert.ok(page.byId('messages').children.some(message => message.textContent.includes('startup timed out')));

  await page.window.send();
  assert.equal(starts(page).length, 2, 'the sending lock was released for retry');
  assert.equal(JSON.parse(prompts(page)[0].body).message, 'accepted draft');
  assert.equal(prompts(page)[0].path, '/api/agent/sessions/retry/prompt');
  finishStalled(json({id: 'late'}));
  await flush();
  assert.equal(deletes(page).filter(call => call.path === '/api/agent/sessions/late').length, 1);
  await page.type('next draft');
  await page.window.send();
  assert.equal(starts(page).length, 2, 'the late response did not replace the claimed session');
  assert.equal(prompts(page).at(-1).path, '/api/agent/sessions/retry/prompt');
});

test('a failed Send startup preserves the accepted draft for retry', async () => {
  let attempts = 0;
  const page = await boot(() => ++attempts === 1 ? Promise.reject(new Error('offline')) : json({id: 'retry'}));
  await page.type('keep this draft');
  await page.window.send();
  assert.equal(page.byId('input').value, 'keep this draft');
  assert.equal(prompts(page).length, 0);
  await page.window.send();
  assert.equal(JSON.parse(prompts(page)[0].body).message, 'keep this draft');
});
