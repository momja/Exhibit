import { test } from 'node:test';
import assert from 'node:assert/strict';
import { requestRelay } from './relay-request.js';

const flush = () => new Promise(setImmediate);
const throttle = seconds => Response.json({ code: 'relay_rate_limited', error: 'Wait before trying again.' }, {
  status: 429, headers: seconds === undefined ? {} : { 'Retry-After': seconds },
});

for (const method of ['GET', 'HEAD']) {
  test(method + ' waits for local Retry-After and resumes', async t => {
    t.mock.timers.enable({ apis: ['Date', 'setTimeout'] });
    let calls = 0;
    const pending = requestRelay(async () => ++calls === 1 ? throttle('2') : Response.json({ status: 200 }), method, new AbortController().signal);
    await flush();
    assert.equal(calls, 1);
    t.mock.timers.tick(1999);
    await flush();
    assert.equal(calls, 1, 'a retry must not spend authority early');
    t.mock.timers.tick(1);
    assert.equal((await pending).result.status, 200);
    assert.equal(calls, 2);
  });
}

test('writes, provider responses, quota failures and permission errors are never replayed', async () => {
  const scenarios = [
    ...['POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS'].map(method => ({ method, response: () => throttle('1') })),
    { response: () => Response.json({ code: 'connection_quota' }, { status: 429, headers: { 'Retry-After': '1' } }) },
    { response: () => Response.json({ code: 'relay_consent_required' }, { status: 403 }) },
    { response: () => Response.json({ status: 429, headers: { 'Retry-After': '1' } }) },
    { response: () => throttle() },
    ...['0', '-1', 'invalid', 'Infinity'].map(value => ({ response: () => throttle(value) })),
  ];
  for (const scenario of scenarios) {
    let calls = 0;
    const expected = scenario.response();
    const { response } = await requestRelay(async () => { calls++; return expected; }, scenario.method || 'GET', new AbortController().signal);
    assert.equal(response, expected);
    assert.equal(calls, 1);
  }
});

test('a policy change on retry is returned immediately', async t => {
  t.mock.timers.enable({ apis: ['Date', 'setTimeout'] });
  let calls = 0;
  const pending = requestRelay(async () => ++calls === 1 ? throttle('1') : Response.json({ code: 'relay_policy_changed' }, { status: 409 }), 'GET', new AbortController().signal);
  await flush();
  t.mock.timers.tick(1000);
  const { response, result } = await pending;
  assert.equal(response.status, 409);
  assert.equal(result.code, 'relay_policy_changed');
  assert.equal(calls, 2);
});

test('abort during backoff cancels the timer and every further send', async t => {
  t.mock.timers.enable({ apis: ['Date', 'setTimeout'] });
  let calls = 0;
  const controller = new AbortController();
  const pending = requestRelay(async () => { calls++; return throttle('1'); }, 'GET', controller.signal);
  await flush();
  controller.abort();
  await assert.rejects(pending, { name: 'AbortError' });
  t.mock.timers.tick(10000);
  await flush();
  assert.equal(calls, 1);
});

test('repeated throttles have a sixty-second retry budget', async t => {
  t.mock.timers.enable({ apis: ['Date', 'setTimeout'] });
  let calls = 0;
  const pending = requestRelay(async () => { calls++; return throttle('30'); }, 'GET', new AbortController().signal);
  await flush();
  t.mock.timers.tick(30000);
  assert.equal((await pending).response.status, 429);
  assert.equal(calls, 2);
});

test('a suspended page cannot send a late retry after its deadline', async t => {
  t.mock.timers.enable({ apis: ['Date', 'setTimeout'] });
  let calls = 0;
  const pending = requestRelay(async () => { calls++; return throttle('1'); }, 'GET', new AbortController().signal);
  await flush();
  t.mock.timers.setTime(Date.now() + 61000);
  t.mock.timers.tick(1000);
  assert.equal((await pending).response.status, 429);
  assert.equal(calls, 1);
});
