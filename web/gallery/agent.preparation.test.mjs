import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';
const asset = name => fileURLToPath(new URL('../../internal/api/assets/gallery/' + name, import.meta.url));
const flush = async () => { for (let i = 0; i < 12; i++) await Promise.resolve(); };
const json = body => ({ok: true, status: 201, json: async () => body});
async function boot(start, {prompt = () => json({}), restore = () => json({}), ticket = () => json({}), byok = false} = {}) {
  const calls = [];
  const streams = [];
  const timers = new Map();
  let next = 0;
  const page = loadPageScript([asset('state-api.js'), asset('network-prompt.js')], {
    TOKEN: '', READ_ONLY: false, BYOK: byok, artifact: {id: 'art-1', title: 'A'}, AbortController,
    apiFetch: async (path, options = {}) => {
      calls.push({path, ...options});
      if (path === '/api/agent/key' && (options.method || 'GET') === 'GET') {
        return json({configured: true, provider: 'openrouter', model: 'test-model'});
      }
      if (path === '/api/agent/sessions') return start(JSON.parse(options.body));
      if (path.endsWith('/prompt')) return prompt(path, JSON.parse(options.body));
      if (path.endsWith('/restore')) return restore(path, options);
      if (path.endsWith('/ticket')) return ticket(path, options);
      return json({});
    },
    apiEventSource: () => { const source = {closed: false, close() { this.closed = true; }}; streams.push(source); return source; },
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
const emit = (page, event) => page.streams.at(-1).onmessage({data: JSON.stringify(event)});
const errors = page => page.byId('messages').children.filter(message => message.className === 'msg err');

test('a provider authentication failure after source preparation appears once when its run ends', async () => {
  const page = await boot(() => json({id: 'session'}), {byok: true});
  await page.type('connection test');
  await page.window.send();
  emit(page, {type: 'agent_start'});
  emit(page, {type: 'exhibit_source_prepared'});
  assert.ok(page.byId('messages').children.some(message => message.textContent === 'Prepared artifact source.'));
  const message = {role: 'assistant', stopReason: 'error', content: [],
    errorMessage: '401: {"message":"User not found.","code":401}'};
  emit(page, {type: 'message_start', message});
  emit(page, {type: 'message_end', message});
  emit(page, {type: 'turn_end', message});
  assert.equal(errors(page).length, 0);
  emit(page, {type: 'agent_end', messages: [message], willRetry: false});
  assert.equal(page.byId('stop-btn').style.display, '', 'the run stays active until agent_settled');
  emit(page, {type: 'agent_settled'});
  assert.equal(page.byId('stop-btn').style.display, 'none');
  assert.equal(errors(page).length, 1);
  assert.match(errors(page)[0].textContent, /provider rejected the API key.*key button above.*try again/);
  await page.type('try again');
  assert.equal(page.byId('send-btn').disabled, false);
});

test('expired and invalid API key errors explain how to recover without an HTTP status', async () => {
  for (const errorMessage of ['Your API key has expired.', 'Invalid API key']) {
    const page = await boot(() => json({id: 'session'}), {byok: true});
    await page.type('connection test');
    await page.window.send();
    emit(page, {type: 'agent_end', messages: [{role: 'assistant', stopReason: 'error', errorMessage}], willRetry: false});
    assert.equal(errors(page).length, 1);
    assert.match(errors(page)[0].textContent, /provider rejected the API key.*key button above.*try again/);
  }
});

test('hosted provider failures show only a generic message, regardless of error detail', async () => {
  for (const errorMessage of [
    '401 Unauthorized',
    'Your API key has expired.',
    'OpenRouter: 503 Service unavailable; API key diagnostic-credential',
    undefined,
  ]) {
    const page = await boot(() => json({id: 'session'}));
    await page.type('connection test');
    await page.window.send();
    emit(page, {type: 'agent_end', messages: [{role: 'assistant', stopReason: 'error', errorMessage}], willRetry: false});
    emit(page, {type: 'agent_settled'});
    assert.equal(errors(page).length, 1);
    assert.equal(errors(page)[0].textContent, 'The agent could not complete your request. Please try again.');
  }
});

test('retryable provider failures stay quiet until retries finish', async () => {
  const page = await boot(() => json({id: 'session'}), {byok: true});
  await page.type('connection test');
  await page.window.send();
  const message = {role: 'assistant', stopReason: 'error', errorMessage: '503 Service unavailable'};
  emit(page, {type: 'agent_end', messages: [message], willRetry: true});
  emit(page, {type: 'auto_retry_start', attempt: 1, maxAttempts: 1});
  assert.equal(errors(page).length, 0);
  emit(page, {type: 'agent_end', messages: [message], willRetry: false});
  emit(page, {type: 'auto_retry_end', success: false, finalError: message.errorMessage});
  emit(page, {type: 'agent_settled'});
  assert.equal(errors(page).length, 1);
  assert.match(errors(page)[0].textContent, /503 Service unavailable/);
});

test('a successful retry leaves no provider failure in chat', async () => {
  const page = await boot(() => json({id: 'session'}));
  await page.type('connection test');
  await page.window.send();
  const failed = {role: 'assistant', stopReason: 'error', errorMessage: '503 Service unavailable'};
  emit(page, {type: 'agent_end', messages: [failed], willRetry: true});
  emit(page, {type: 'auto_retry_start', attempt: 1, maxAttempts: 1});
  const succeeded = {role: 'assistant', stopReason: 'stop'};
  emit(page, {type: 'agent_end', messages: [failed, succeeded], willRetry: false});
  emit(page, {type: 'auto_retry_end', success: true});
  emit(page, {type: 'agent_settled'});
  assert.equal(errors(page).length, 0);
});

test('successes, cancellations, and tool errors are not provider failures', async () => {
  for (const finalMessage of [
    {role: 'assistant', stopReason: 'stop'},
    {role: 'assistant', stopReason: 'aborted', errorMessage: 'Aborted'},
    {role: 'toolResult', isError: true, errorMessage: 'Tool failed'},
  ]) {
    const page = await boot(() => json({id: 'session'}));
    await page.type('connection test');
    await page.window.send();
    emit(page, {type: 'agent_end', messages: [finalMessage], willRetry: false});
    emit(page, {type: 'agent_settled'});
    assert.equal(errors(page).length, 0);
  }
});

test('provider failures without detail still give a useful message', async () => {
  const page = await boot(() => json({id: 'session'}), {byok: true});
  await page.type('connection test');
  await page.window.send();
  emit(page, {type: 'agent_end', messages: [{role: 'assistant', stopReason: 'error'}], willRetry: false});
  emit(page, {type: 'agent_settled'});
  assert.equal(errors(page).length, 1);
  assert.match(errors(page)[0].textContent, /provider could not complete the request/);
});

test('composer button state follows draft changes and Send while preparation is active', async () => {
  const page = await boot(() => json({id: 'ready'}));
  await page.type('   ');
  assert.equal(page.byId('send-btn').disabled, true);
  await page.type('change the heading');
  assert.equal(page.byId('send-btn').disabled, false);
  await page.tick(300);
  assert.equal(starts(page).length, 1);
  assert.equal(page.byId('send-btn').disabled, false);
  await page.window.send();
  assert.equal(prompts(page).length, 1);
  assert.equal(page.byId('input').value, '');
  assert.equal(page.byId('send-btn').disabled, true);
});

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

test('navigation during a History startup ignores its late response without deleting the session', async () => {
 let resolve;
 const page=await boot(()=>new Promise(r=>{resolve=r;}));
 // Delegated fragment buttons use the same module-scoped resume handler.
 const button={dataset:{resume:'old-conversation',rollback:'0'}};
 const resuming=page.byId('history').click({closest:()=>button});await flush();
 await page.dispatchOnWindow('pagehide');resolve(json({id:'resumed'}));
 await resuming;await flush();
 assert.equal(deletes(page).length,0);assert.equal(prompts(page).length,0);
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
  assert.equal(deletes(page).filter(call => call.path === '/api/agent/sessions/old-conversation').length, 0);
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
  assert.equal(deletes(page).filter(call => call.path.endsWith('/old-conversation')).length, 0);
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
  assert.equal(deletes(page).filter(call => call.path.endsWith('/old-conversation')).length, 0);
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
  assert.equal(deletes(page).filter(call => call.path.endsWith('/old-conversation')).length, 0);
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
    assert.equal(deletes(page).length, 0, 'only the server lease can reclaim abandoned History sessions');
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

for (const ending of ['timeout', 'invalidation']) {
  test(`a late 201 History response after ${ending} preserves the session claimed in another browser context`, async () => {
    let finishOriginal;
    let claimed = false;
    const original = await boot(() => new Promise(resolve => { finishOriginal = resolve; }));
    const other = await boot(() => ({ok: true, status: 200, json: async () => ({id: 'shared'})}), {
      prompt: () => { claimed = true; return json({}); }
    });
    await original.type('original unsent draft');
    const pending = original.resume('shared', 0);
    await flush();
    if (ending === 'invalidation') original.window.openHistory();
    await other.resume('shared', 0);
    await other.type('claim in the other tab');
    await other.window.send();
    assert.equal(claimed, true, 'an accepted prompt has canceled the server preparation lease');
    if (ending === 'timeout') await original.tick(90000);
    await pending;
    finishOriginal(json({id: 'shared'}));
    await flush();
    assert.equal(deletes(original).length, 0, 'even a created-201 response grants no authority to delete a now-claimed History session');
    assert.equal(original.streams.length, 0);
    assert.equal(original.byId('input').value, 'original unsent draft');
    assert.equal(other.streams[0].closed, false);
    await other.type('still active');
    await other.window.send();
    assert.equal(prompts(other).at(-1).path, '/api/agent/sessions/shared/prompt');
  });
}

const restores = page => page.calls.filter(call => call.path.endsWith('/restore'));

test('a stalled rollback times out, detaches locally, and unlocks Send without retrying the uncertain restore', async () => {
  let finishRestore;
  const page = await boot(body => json({id: body.resume_session_id || 'fresh'}), {
    restore: () => new Promise(resolve => { finishRestore = resolve; })
  });
  await page.type('preserved draft');
  page.byId('messages').innerHTML = '<div>previous chat</div>';
  page.byId('history-messages').innerHTML = '<div>old history transcript</div>';
  const pending = page.resume('old-conversation', 2);
  await flush();
  assert.equal(restores(page).length, 1);
  assert.equal(restores(page)[0].path, '/api/artifacts/art-1/versions/2/restore');
  assert.equal(page.streams[0].closed, false);
  await page.tick(90000);
  await pending;
  assert.match(page.byId('resume-status').textContent, /may have completed/);
  assert.match(page.byId('resume-status').textContent, /not continued/);
  assert.equal(page.byId('messages').innerHTML, '<div>previous chat</div>');
  assert.equal(page.byId('input').value, 'preserved draft');
  assert.equal(page.streams[0].closed, true);
  assert.equal(deletes(page).length, 0, 'local detach leaves a potentially claimed sidecar running');
  await page.window.send();
  assert.equal(prompts(page)[0].path, '/api/agent/sessions/fresh/prompt');
  assert.equal(JSON.parse(prompts(page)[0].body).message, 'preserved draft');
  assert.equal(restores(page).length, 1, 'an uncertain restore is never automatically retried');
  finishRestore(json({}));
  await flush();
  assert.ok(!page.byId('messages').innerHTML.includes('old history transcript'));
  assert.equal(page.streams[1].closed, false);
});

for (const ending of ['timeout', 'invalidation']) {
  for (const lateSuccess of [true, false]) {
    test(`late rollback ${lateSuccess ? 'success' : 'failure'} after ${ending} cannot install history or unlock a newer continuation`, async () => {
      let finishOld;
      let finishNew;
      const page = await boot(body => json({id: body.resume_session_id}), {
        restore: path => new Promise(resolve => {
          if (path.includes('/versions/2/')) finishOld = resolve;
          else finishNew = resolve;
        })
      });
      await page.type('unsent draft');
      page.byId('messages').innerHTML = '<div>existing chat</div>';
      page.byId('history-messages').innerHTML = '<div>old transcript</div>';
      const old = page.resume('old-conversation', 2);
      await flush();
      if (ending === 'timeout') await page.tick(90000);
      else page.window.openHistory();
      await old;
      page.byId('history-messages').innerHTML = '<div>new transcript</div>';
      const next = page.resume('new-conversation', 3);
      await flush();
      finishOld(lateSuccess ? json({})
        : {ok: false, status: 500, json: async () => ({error: 'late old restore refusal'})});
      await flush();
      await page.resume('third-conversation', 0);
      await page.window.send();
      assert.equal(starts(page).length, 2, 'only the new continuation owns the lock');
      assert.equal(prompts(page).length, 0);
      assert.equal(restores(page).length, 2, 'neither restore was retried');
      assert.equal(page.byId('messages').innerHTML, '<div>existing chat</div>');
      assert.equal(page.byId('resume-status').textContent, 'Starting…');
      assert.equal(page.byId('input').value, 'unsent draft');
      assert.equal(page.streams[0].closed, true);
      assert.equal(page.streams[1].closed, false);
      assert.equal(deletes(page).length, 0);
      finishNew(json({}));
      await next;
      assert.ok(page.byId('messages').innerHTML.includes('new transcript'));
      assert.ok(!page.byId('messages').innerHTML.includes('old transcript'));
      await page.window.send();
      assert.equal(prompts(page)[0].path, '/api/agent/sessions/new-conversation/prompt');
    });
  }
}

test('server session_closed during rollback releases its continuation and ignores the late restore', async () => {
  let finishRestore;
  const page = await boot(body => json({id: body.resume_session_id}), {
    restore: () => new Promise(resolve => { finishRestore = resolve; })
  });
  await page.type('preserved draft');
  page.byId('history-messages').innerHTML = '<div>old transcript</div>';
  const old = page.resume('old-conversation', 2);
  await flush();
  page.streams[0].onmessage({data: JSON.stringify({type: 'exhibit_session_closed'})});
  await old;
  assert.match(page.byId('resume-status').textContent, /may have completed/);
  assert.equal(page.streams[0].closed, true);
  assert.equal(deletes(page).length, 0);
  page.byId('history-messages').innerHTML = '<div>new transcript</div>';
  await page.resume('new-conversation', 0);
  finishRestore(json({}));
  await flush();
  assert.ok(page.byId('messages').innerHTML.includes('new transcript'));
  assert.ok(!page.byId('messages').innerHTML.includes('old transcript'));
  await page.window.send();
  assert.equal(prompts(page)[0].path, '/api/agent/sessions/new-conversation/prompt');
  assert.equal(restores(page).length, 1);
});

for (const invalidation of ['navigation', 'key reset']) {
  test(`${invalidation} during rollback detaches the candidate without deleting a sidecar another tab could claim`, async () => {
    let finishRestore;
    const page = await boot(body => json({id: body.resume_session_id || 'fresh'}), {
      restore: () => new Promise(resolve => { finishRestore = resolve; })
    });
    await page.type('preserved draft');
    const old = page.resume('old-conversation', 2);
    await flush();
    if (invalidation === 'navigation') {
      await page.dispatchOnWindow('pagehide');
      await page.dispatchOnWindow('pageshow');
    } else {
      page.byId('key-provider').value = 'openai';
      page.byId('key-secret').value = 'new-key';
      await page.window.saveKey();
    }
    await old;
    assert.equal(deletes(page).length, 0);
    assert.equal(page.streams[0].closed, true);
    await page.window.send();
    assert.equal(prompts(page)[0].path, '/api/agent/sessions/fresh/prompt');
    finishRestore(json({}));
    await flush();
    assert.equal(page.streams[1].closed, false);
    assert.equal(restores(page).length, 1);
  });
}

test('closure of a previous live session does not cancel a newer History startup', async () => {
  let finishHistory;
  const page = await boot(body => body.resume_session_id
    ? new Promise(resolve => { finishHistory = resolve; }) : json({id: 'previous'}));
  await page.type('initial prompt');
  await page.window.send();
  const next = page.resume('next-conversation', 0);
  await flush();
  page.streams[0].onmessage({data: JSON.stringify({type: 'exhibit_session_closed'})});
  await page.resume('third-conversation', 0);
  assert.equal(starts(page).length, 2, 'the new startup remains the owner of the History lock');
  finishHistory(json({id: 'next-conversation'}));
  await next;
  await page.type('next prompt');
  await page.window.send();
  assert.equal(prompts(page).at(-1).path, '/api/agent/sessions/next-conversation/prompt');
});

test('a stalled rollback error body is bounded and late parsing cannot replace a newer transcript', async () => {
  let finishErrorBody;
  const page = await boot(body => json({id: body.resume_session_id}), {
    restore: () => ({ok: false, status: 500, json: () => new Promise(resolve => { finishErrorBody = resolve; })})
  });
  const old = page.resume('old-conversation', 2);
  await flush();
  await page.tick(90000);
  await old;
  assert.match(page.byId('resume-status').textContent, /may have completed/);
  page.byId('history-messages').innerHTML = '<div>new transcript</div>';
  await page.resume('new-conversation', 0);
  finishErrorBody({error: 'late old refusal'});
  await flush();
  assert.ok(page.byId('messages').innerHTML.includes('new transcript'));
  assert.equal(page.byId('resume-status').textContent, 'Starting…');
  assert.equal(deletes(page).length, 0);
  assert.equal(restores(page).length, 1);
});

test('a server 404 on SSE reconnection releases pending rollback just like session_closed', async () => {
  let finishRestore;
  const page = await boot(body => json({id: body.resume_session_id || 'fresh'}), {
    restore: () => new Promise(resolve => { finishRestore = resolve; }),
    ticket: () => ({ok: false, status: 404, json: async () => ({error: 'session expired'})})
  });
  const old = page.resume('old-conversation', 2);
  await flush();
  page.streams[0].onerror();
  await page.tick(1000);
  await old;
  assert.match(page.byId('resume-status').textContent, /may have completed/);
  await page.type('new prompt');
  await page.window.send();
  assert.equal(prompts(page)[0].path, '/api/agent/sessions/fresh/prompt');
  finishRestore(json({}));
  await flush();
  assert.equal(deletes(page).length, 0);
  assert.equal(restores(page).length, 1);
});

test('late rollback cannot detach a newer attach to the same historical session id', async () => {
  let finishRestore;
  const page = await boot(body => json({id: body.resume_session_id}), {
    restore: () => new Promise(resolve => { finishRestore = resolve; })
  });
  const old = page.resume('same-conversation', 2);
  await flush();
  page.window.openHistory();
  const next = page.resume('same-conversation', 0);
  await old;
  await next;
  assert.equal(page.streams[0].closed, true);
  assert.equal(page.streams[1].closed, false);
  finishRestore(json({}));
  await flush();
  assert.equal(page.streams[1].closed, false);
  await page.type('continue here');
  await page.window.send();
  assert.equal(prompts(page)[0].path, '/api/agent/sessions/same-conversation/prompt');
  assert.equal(deletes(page).length, 0);
});
