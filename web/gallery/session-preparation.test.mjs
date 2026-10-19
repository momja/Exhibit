import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createSessionPreparation, createTypingPreparation } from './session-preparation.js';

function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

const flush = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); };
function timers() {
  const callbacks = new Map();
  let nextId = 0;
  let time = 0;
  async function advance(milliseconds) {
    const target = time + milliseconds;
    for (;;) {
      const next = [...callbacks].sort((a, b) => a[1].at - b[1].at)[0];
      if (!next || next[1].at > target) break;
      time = next[1].at;
      callbacks.delete(next[0]);
      next[1].fn();
      await flush();
    }
    time = target;
    await flush();
  }
  return {
    now: () => time,
    schedule(fn, delay = 0) { callbacks.set(++nextId, {fn, at: time + delay}); return nextId; },
    cancel(id) { callbacks.delete(id); },
    advance,
    // Simulate a suspended page whose deadline timer hasn't been dispatched.
    elapse(milliseconds) { time += milliseconds; },
    async tick() {
      const until = Math.max(time, ...[...callbacks.values()].map(timer => timer.at));
      await advance(until - time);
    },
  };
}

test('Send claims an in-flight eager startup rather than starting a second session', async () => {
  const startup = deferred();
  let starts = 0;
  const closed = [];
  const clock = timers();
  const preparation = createSessionPreparation({ start: () => { starts++; return startup.promise; },
    close: value => closed.push(value), ...clock });
  const eager = preparation.prepare();
  const sent = preparation.claim();
  startup.resolve({ id: 'one' });
  assert.equal((await eager).id, 'one');
  assert.equal((await sent).id, 'one');
  assert.equal(starts, 1);
  await clock.tick();
  assert.deepEqual(closed, [], 'a claimed session does not expire');
  preparation.release();
});

test('unused preparations expire and a later Send starts afresh', async () => {
  let starts = 0;
  const closed = [];
  const clock = timers();
  const preparation = createSessionPreparation({ start: async () => ({ id: ++starts }),
    close: value => closed.push(value.id), ...clock });
  await preparation.prepare();
  await clock.tick();
  assert.deepEqual(closed, [1]);
  assert.equal((await preparation.claim()).id, 2);
  preparation.release();
});

test('navigation disposes a late response without disturbing a newer startup', async () => {
  const old = deferred();
  const current = deferred();
  const requests = [old, current];
  const closed = [];
  const clock = timers();
  const preparation = createSessionPreparation({ start: () => requests.shift().promise,
    close: value => closed.push(value.id), ...clock });
  const first = preparation.prepare();
  await Promise.resolve();
  preparation.discard();
  const second = preparation.claim();
  await Promise.resolve();
  old.resolve({ id: 'old' });
  assert.equal(await first, null);
  current.resolve({ id: 'current' });
  assert.equal((await second).id, 'current');
  assert.deepEqual(closed, ['old']);
  preparation.release();
});

test('a failed speculative startup can be retried by Send', async () => {
  let starts = 0;
  const clock = timers();
  const preparation = createSessionPreparation({ start: async () => {
    if (++starts === 1) throw new Error('temporarily unavailable');
    return { id: 'retry' };
  }, close() {}, ...clock });
  await assert.rejects(preparation.prepare(), /unavailable/);
  assert.equal((await preparation.claim()).id, 'retry');
  preparation.release();
});

test('typing requires a sustained deliberate draft and handles a paste', async () => {
  const clock = timers();
  let starts = 0;
  let ready = true;
  const typing = createTypingPreparation({ prepare: () => { starts++; }, ready: () => ready, ...clock });
  typing.update('a');
  await clock.tick();
  typing.update('a b');
  await clock.tick();
  assert.equal(starts, 0);
  typing.update('abc');
  typing.update('');
  await clock.tick();
  assert.equal(starts, 0, 'deleting the draft cancels preparation');
  typing.update('pasted draft');
  await clock.tick();
  assert.equal(starts, 1);
  typing.update('another draft');
  ready = false;
  await clock.tick();
  assert.equal(starts, 1, 'recheck availability when the timer fires');
});

test('discarding before the startup microtask runs makes no session request', async () => {
 let starts=0;
 const preparation=createSessionPreparation({start:async()=>{starts++;return{id:'unused'};},close(){}});
 const pending=preparation.prepare();preparation.discard();
 assert.equal(await pending,null);assert.equal(starts,0);
});


test('a stalled POST claimed by Send times out, permits retry, and disposes its late success', async () => {
  const stalled = deferred();
  const closed = [];
  let starts = 0;
  const clock = timers();
  const preparation = createSessionPreparation({ start: () => ++starts === 1 ? stalled.promise : {id: 'new'},
    close: session => closed.push(session.id), expireAfter: 90, ...clock });
  const eager = preparation.prepare();
  await flush();
  await clock.advance(60);
  const claimed = preparation.claim();
  assert.equal(claimed, eager, 'Send reuses the same bounded startup');
  const rejected = assert.rejects(claimed, /startup timed out/);
  await clock.advance(30);
  await rejected;
  assert.equal(starts, 1);
  assert.equal((await preparation.claim()).id, 'new');
  stalled.resolve({id: 'late'});
  await flush();
  assert.deepEqual(closed, ['late']);
  assert.equal((await preparation.claim()).id, 'new', 'late completion cannot replace the new session');
  preparation.release();
});

test('a delayed ready response gets only the remainder of the preparation deadline', async () => {
  const startup = deferred();
  const closed = [];
  const clock = timers();
  const preparation = createSessionPreparation({start: () => startup.promise,
    close: session => closed.push(session.id), expireAfter: 90, ...clock});
  const pending = preparation.prepare();
  await clock.advance(70);
  startup.resolve({id: 'slow'});
  await pending;
  await clock.advance(19);
  assert.deepEqual(closed, []);
  await clock.advance(1);
  assert.deepEqual(closed, ['slow'], 'ready time does not renew the original deadline');
});

test('a ready claim cancels unused expiry and follows normal caller lifetime', async () => {
  const closed = [];
  const clock = timers();
  const preparation = createSessionPreparation({start: () => ({id: 'ready'}),
    close: session => closed.push(session.id), expireAfter: 90, ...clock});
  await preparation.prepare();
  await clock.advance(89);
  assert.equal((await preparation.claim()).id, 'ready');
  await clock.advance(1000);
  assert.equal((await preparation.claim()).id, 'ready');
  assert.deepEqual(closed, []);
  preparation.release();
});

test('navigation settles a pending claim without waiting for the POST', async () => {
  const startup = deferred();
  const closed = [];
  const clock = timers();
  const preparation = createSessionPreparation({start: () => startup.promise,
    close: session => closed.push(session.id), ...clock});
  const pending = preparation.claim();
  await flush();
  preparation.discard();
  assert.equal(await pending, null);
  startup.resolve({id: 'late'});
  await flush();
  assert.deepEqual(closed, ['late']);
});

test('a response after the deadline cannot cancel an overdue timer when Send is waiting', async () => {
  const startup = deferred();
  const closed = [];
  const clock = timers();
  const preparation = createSessionPreparation({start: () => startup.promise,
    close: session => closed.push(session.id), expireAfter: 90, ...clock});
  const pending = preparation.claim();
  const rejected = assert.rejects(pending, /startup timed out/);
  await flush();
  clock.elapse(91);
  startup.resolve({id: 'already-expired'});
  await rejected;
  assert.deepEqual(closed, ['already-expired']);
});

test('claiming an overdue ready session starts afresh even before its timer callback runs', async () => {
  let starts = 0;
  const closed = [];
  const clock = timers();
  const preparation = createSessionPreparation({start: () => ({id: ++starts}),
    close: session => closed.push(session.id), expireAfter: 90, ...clock});
  await preparation.prepare();
  clock.elapse(91);
  assert.equal((await preparation.claim()).id, 2);
  assert.deepEqual(closed, [1]);
  preparation.release();
});

test('a timed-out request rejecting late cannot disturb a newer pending claim', async () => {
  const old = deferred();
  const next = deferred();
  const clock = timers();
  let starts = 0;
  const preparation = createSessionPreparation({start: () => ++starts === 1 ? old.promise : next.promise,
    close() {}, expireAfter: 90, ...clock});
  const rejected = assert.rejects(preparation.claim(), /startup timed out/);
  await flush();
  await clock.advance(90);
  await rejected;
  const pending = preparation.claim();
  await flush();
  old.reject(new Error('late network failure'));
  await flush();
  assert.equal(preparation.claim(), pending);
  next.resolve({id: 'next'});
  assert.equal((await pending).id, 'next');
  await clock.advance(1000);
  assert.equal((await preparation.claim()).id, 'next');
  preparation.release();
});
