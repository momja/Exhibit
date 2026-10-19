import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createSessionPreparation, createTypingPreparation } from './session-preparation.js';

function deferred() {
  let resolve, reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function timers() {
  const callbacks = new Map();
  let nextId = 0;
  return {
    schedule(fn) { callbacks.set(++nextId, fn); return nextId; },
    cancel(id) { callbacks.delete(id); },
    async tick() {
      const pending = [...callbacks.values()];
      callbacks.clear();
      for (const callback of pending) callback();
      await Promise.resolve();
      await Promise.resolve();
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
