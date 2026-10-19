import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';
const asset = name => fileURLToPath(new URL('../../internal/api/assets/gallery/' + name, import.meta.url));
const flush = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); };
const json = body => ({ok: true, status: 201, json: async () => body});
async function boot(start, {prompt = () => json({})} = {}) {
  const calls = [];
  const streams = [];
  const timers = new Map();
  let next = 0;
  const page = loadPageScript([asset('state-api.js'), asset('network-prompt.js')], {
    TOKEN: '', READ_ONLY: false, BYOK: false, artifact: {id: 'art-1', title: 'A'},
    apiFetch: async (path, options = {}) => {
      calls.push({path, ...options});
      if (path === '/api/agent/sessions') return start(JSON.parse(options.body));
      if (path.endsWith('/prompt')) return prompt(path, JSON.parse(options.body));
      return json({});
    },
    apiEventSource: () => { const source = {close() {}}; streams.push(source); return source; },
    setTimeout: (fn, delay) => { const id = ++next; timers.set(id, {fn, delay}); return id; },
    clearTimeout: id => timers.delete(id),
    matchMedia: () => ({matches: false, addEventListener() {}}),
    CustomEvent: class { constructor(type) { this.type = type; } },
  });
  const {resumeConversation} = await page.loadModule(asset('agent.js'), ['resumeConversation']);
  await flush();
  const tick = async delay => {
    for (const [id, timer] of [...timers]) if (timer.delay === delay) { timers.delete(id); timer.fn(); }
    await flush();
  };
  const type = async text => { page.byId('input').value = text; await page.byId('input').dispatchEvent({type: 'input'}); };
  return {...page, calls, streams, tick, type, resume: resumeConversation};
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

test('a stalled History POST times out after 90 seconds and unlocks Send without losing the draft', async () => {
  let finishHistory;
  const page = await boot(body => body.resume_session_id
    ? new Promise(resolve => { finishHistory = resolve; }) : json({id: 'fresh'}));
  await page.type('keep this draft');
  const resuming = page.resume('old-conversation', 2);
  await flush();
  assert.equal(starts(page).length, 1);
  assert.deepEqual(JSON.parse(starts(page)[0].body), {artifact_id: 'art-1', resume_session_id: 'old-conversation'});
  await page.tick(90000);
  await resuming;
  assert.match(page.byId('resume-status').textContent, /startup timed out/);
  assert.equal(page.byId('input').value, 'keep this draft');
  assert.equal(page.calls.filter(call => call.path.endsWith('/restore')).length, 0);

  await page.window.send();
  assert.equal(starts(page).length, 2);
  assert.deepEqual(JSON.parse(starts(page)[1].body), {artifact_id: 'art-1'});
  assert.equal(prompts(page)[0].path, '/api/agent/sessions/fresh/prompt');
  assert.equal(JSON.parse(prompts(page)[0].body).message, 'keep this draft');
  finishHistory(json({id: 'old-conversation'}));
  await flush();
  assert.ok(deletes(page).some(call => call.path === '/api/agent/sessions/old-conversation'));
  assert.equal(page.streams.length, 1, 'late History startup cannot attach over Send');
});

test('a stalled History POST times out and a new resume keeps its own transcript and request', async () => {
  let finishOld;
  let attempts = 0;
  const page = await boot(() => ++attempts === 1
    ? new Promise(resolve => { finishOld = resolve; }) : json({id: 'new-conversation'}));
  await page.type('unsent draft');
  const old = page.resume('old-conversation', 0);
  await flush();
  await page.tick(90000);
  await old;
  page.byId('history-messages').innerHTML = '<div>the chosen conversation</div>';
  const next = page.resume('new-conversation', 0);
  page.byId('history-messages').innerHTML = '<div>a later fragment</div>';
  await next;
  assert.deepEqual(JSON.parse(starts(page)[1].body), {artifact_id: 'art-1', resume_session_id: 'new-conversation'});
  assert.ok(page.byId('messages').innerHTML.includes('the chosen conversation'));
  assert.ok(!page.byId('messages').innerHTML.includes('a later fragment'));
  assert.equal(page.byId('input').value, 'unsent draft');
  finishOld(json({id: 'old-conversation'}));
  await flush();
  assert.ok(deletes(page).some(call => call.path.endsWith('/old-conversation')));
  assert.equal(page.streams.length, 1);
});

test('navigation invalidates pending History immediately and restoration can Send before its response', async () => {
  let finishOld;
  const page = await boot(body => body.resume_session_id
    ? new Promise(resolve => { finishOld = resolve; }) : json({id: 'fresh'}));
  await page.type('draft after restoring navigation');
  const old = page.resume('old-conversation', 0);
  await flush();
  await page.dispatchOnWindow('pagehide');
  await old;
  await page.dispatchOnWindow('pageshow');
  await page.window.send();
  assert.equal(prompts(page)[0].path, '/api/agent/sessions/fresh/prompt');
  assert.equal(JSON.parse(prompts(page)[0].body).message, 'draft after restoring navigation');
  finishOld(json({id: 'old-conversation'}));
  await flush();
  assert.ok(deletes(page).some(call => call.path.endsWith('/old-conversation')));
});

test('invalidated History completion and finally cannot unlock a newer pending History request', async () => {
  let finishOld;
  let finishNew;
  const page = await boot(body => new Promise(resolve => {
    if (body.resume_session_id === 'old-conversation') finishOld = resolve;
    else finishNew = resolve;
  }));
  await page.type('untouched draft');
  const old = page.resume('old-conversation', 0);
  await flush();
  page.window.openHistory();
  const next = page.resume('new-conversation', 0);
  await old;
  await flush();
  finishOld(json({id: 'old-conversation'}));
  await flush();
  const status = page.byId('resume-status').textContent;
  await page.resume('third-conversation', 0);
  await page.window.send();
  assert.equal(starts(page).length, 2, 'the newer attempt still owns the switching lock');
  assert.equal(prompts(page).length, 0);
  assert.equal(page.streams.length, 0, 'the old conversation was never attached');
  assert.equal(page.byId('input').value, 'untouched draft');
  assert.equal(status, 'Starting…', 'old completion does not overwrite the new request status');
  assert.ok(deletes(page).some(call => call.path.endsWith('/old-conversation')));
  finishNew(json({id: 'new-conversation'}));
  await next;
  await page.window.send();
  assert.equal(prompts(page)[0].path, '/api/agent/sessions/new-conversation/prompt');
});

test('a key reset releases pending History and its late rejection cannot affect a newer resume', async () => {
  let rejectOld;
  const page = await boot(body => body.resume_session_id === 'old-conversation'
    ? new Promise((_resolve, reject) => { rejectOld = reject; }) : json({id: 'new-conversation'}));
  await page.type('preserved draft');
  const old = page.resume('old-conversation', 0);
  await flush();
  page.byId('key-provider').value = 'openai';
  page.byId('key-secret').value = 'new-key';
  await page.window.saveKey();
  await old;
  await page.resume('new-conversation', 0);
  rejectOld(new Error('old network failure'));
  await flush();
  assert.equal(page.byId('input').value, 'preserved draft');
  assert.equal(page.byId('resume-status').textContent, 'Starting…');
  await page.window.send();
  assert.equal(prompts(page)[0].path, '/api/agent/sessions/new-conversation/prompt');
});

for (const invalidation of ['navigation', 'key reset']) {
  test(`a stalled prompt POST invalidated by ${invalidation} cannot hold or release a newer Send lock`, async () => {
    let finishOld;
    let finishNew;
    let sessionCount = 0;
    let promptCount = 0;
    const page = await boot(() => json({id: 'session-' + (++sessionCount)}), {
      prompt: () => ++promptCount === 1 ? new Promise(resolve => { finishOld = resolve; })
        : promptCount === 2 ? new Promise(resolve => { finishNew = resolve; }) : json({})
    });
    await page.type('first accepted prompt');
    const oldSend = page.window.send();
    await flush();
    assert.equal(prompts(page).length, 1);
    assert.equal(prompts(page)[0].path, '/api/agent/sessions/session-1/prompt');
    if (invalidation === 'navigation') {
      await page.dispatchOnWindow('pagehide');
      await page.dispatchOnWindow('pageshow');
    } else {
      page.byId('key-provider').value = 'openai';
      page.byId('key-secret').value = 'new-key';
      await page.window.saveKey();
    }
    await page.type('second accepted prompt');
    const newSend = page.window.send();
    await flush();
    assert.equal(prompts(page).length, 2, 'invalidation released the old pending prompt lock');
    assert.equal(prompts(page)[1].path, '/api/agent/sessions/session-2/prompt');
    assert.equal(JSON.parse(prompts(page)[1].body).message, 'second accepted prompt');

    finishOld({ok: false, status: 500, json: async () => ({error: 'stale prompt failure'})});
    await oldSend;
    await page.type('third draft');
    await page.window.send();
    assert.equal(prompts(page).length, 2, 'old finally cannot unlock the newer pending Send');
    assert.equal(page.byId('input').value, 'third draft');
    assert.ok(!page.byId('messages').children.some(message => message.textContent.includes('stale prompt failure')));
    assert.equal(prompts(page).filter(call => JSON.parse(call.body).message === 'first accepted prompt').length, 1,
      'an uncertain accepted prompt is never automatically retried');

    finishNew(json({}));
    await newSend;
    await page.window.send();
    assert.equal(prompts(page).length, 3);
    assert.equal(prompts(page)[2].path, '/api/agent/sessions/session-2/prompt');
    assert.equal(JSON.parse(prompts(page)[2].body).message, 'third draft');
  });
}

for (const retrySucceeds of [true, false]) {
  test(`a late History attach is protected during a retry of the same conversation (${retrySucceeds ? 'success' : 'failure'})`, async () => {
    let finishOld;
    let finishNew;
    let count = 0;
    const page = await boot(() => new Promise(resolve => {
      if (++count === 1) finishOld = resolve;
      else finishNew = resolve;
    }));
    const old = page.resume('same-conversation', 0);
    await flush();
    await page.tick(90000);
    await old;
    const next = page.resume('same-conversation', 0);
    await flush();
    finishOld(json({id: 'same-conversation'}));
    await flush();
    assert.equal(deletes(page).length, 0, 'late cleanup cannot kill the newer pending attach');
    finishNew(retrySucceeds ? json({id: 'same-conversation'})
      : {ok: false, status: 500, json: async () => ({error: 'retry failed'})});
    await next;
    assert.equal(deletes(page).length, retrySucceeds ? 0 : 1, 'an abandoned session is closed if the retry fails');
    assert.equal(page.streams.length, retrySucceeds ? 1 : 0);
  });
}

for (const ending of ['timeout', 'invalidation']) {
  test(`a late 200 History attachment after ${ending} never deletes another tab's live session`, async () => {
    let finishAttach;
    const page = await boot(body => body.resume_session_id
      ? new Promise(resolve => { finishAttach = resolve; }) : json({id: 'fresh'}));
    await page.type('preserved draft');
    const old = page.resume('already-live', 0);
    await flush();
    if (ending === 'timeout') await page.tick(90000);
    else page.window.openHistory();
    await old;
    await page.window.send();
    assert.equal(prompts(page)[0].path, '/api/agent/sessions/fresh/prompt');
    finishAttach({ok: true, status: 200, json: async () => ({id: 'already-live', sse_ticket: 'unused'})});
    await flush();
    assert.equal(deletes(page).filter(call => call.path.endsWith('/already-live')).length, 0);
    assert.equal(page.streams.length, 1, 'the stale attachment was ignored without binding it');
    await page.type('next prompt');
    await page.window.send();
    assert.equal(prompts(page).at(-1).path, '/api/agent/sessions/fresh/prompt');
  });
}
