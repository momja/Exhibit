import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const script = fileURLToPath(new URL('../../internal/api/assets/gallery/agent-questions.js', import.meta.url));
const json = (status, body = {}) => ({ ok: status < 400, status, json: async () => body });

async function fixture(responses = [json(204)]) {
  const calls = [], swaps = [], cards = new Map();
  const fields = [{ disabled: false }, { disabled: false }, { disabled: false }];
  const status = { textContent: '' };
  const entries = [
    { id: 'view', choices: ['Weekly'], text: '' },
    { id: 'goal', choices: [], text: '  Meals for the family  ' }
  ];
  const card = { dataset: { sessionId: 'session-1', requestId: 'request-1', pending: 'true' } };
  const form = {
    dataset: {},
    closest(selector) { return selector === '[data-question-form]' ? form : card; },
    querySelector() { return status; },
    querySelectorAll(selector) {
      if (selector === 'input, textarea, button') return fields;
      return entries.map(entry => ({
        dataset: { questionId: entry.id },
        querySelector: () => ({ value: entry.text }),
        querySelectorAll: () => entry.choices.map(value => ({ value }))
      }));
    }
  };
  const apiFetch = async (path, options = {}) => {
    calls.push({ path, ...options });
    if (options.method === 'GET') return json(200);
    const next = responses.shift();
    if (next instanceof Error) throw next;
    return next || json(204);
  };
  const page = loadPageScript([], { apiFetch, htmx: {
    async ajax(verb, path, options) {
      swaps.push({ verb, path, ...options });
      await options.fetch(path, { method: verb });
      cards.set('agent-question-request-1', card);
    }
  } });
  const byId = page.context.document.getElementById;
  page.context.document.getElementById = id => id.startsWith('agent-question-') ? cards.get(id) || null : byId(id);
  page.byId('messages').querySelectorAll = () => [form];
  await page.loadModule(script);
  const event = (type, extra = {}) => page.context.document.dispatchEvent({
    type: 'exhibit:agent-event', detail: { sessionId: 'session-1', event: { type, requestId: 'request-1', ...extra } }
  });
  const submit = () => page.byId('messages').dispatchEvent({ type: 'submit', target: form, preventDefault() {} });
  const skip = () => page.byId('messages').click({ closest: selector => selector === '[data-question-skip]' ? { closest: () => form } : null });
  return { ...page, calls, swaps, cards, fields, status, entries, form, event, submit, skip };
}

test('question cards use credentialed htmx swaps and reconnect keeps an unfinished draft', async () => {
  const p = await fixture();
  await p.event('exhibit_questions', { pending: true });
  assert.equal(p.swaps.length, 1);
  assert.equal(p.swaps[0].swap, 'beforeend');
  assert.equal(p.calls[0].path, '/api/agent/sessions/session-1/questions/request-1');
  await p.event('exhibit_questions', { pending: true });
  assert.equal(p.swaps.length, 1);
  assert.equal(p.entries[1].text, '  Meals for the family  ');
  await p.event('exhibit_questions', { pending: false });
  assert.equal(p.swaps[1].swap, 'outerHTML');
});

test('one submission sends choices and free-form answers for the complete batch', async () => {
  const p = await fixture();
  await p.submit();
  const post = p.calls.find(call => call.method === 'POST');
  assert.deepEqual(JSON.parse(post.body), { answers: [
    { id: 'view', selected: ['Weekly'], text: '' },
    { id: 'goal', selected: [], text: 'Meals for the family' }
  ], cancelled: false });
  assert.equal(p.fields.every(field => field.disabled), true);
});

test('an option question can be answered entirely in the person’s own words', async () => {
  const p = await fixture();
  p.entries[0].choices = [];
  p.entries[0].text = 'A monthly calendar';
  await p.submit();
  assert.equal(JSON.parse(p.calls[0].body).answers[0].text, 'A monthly calendar');
});

test('the person can clear an earlier choice and switch to their own answer', async () => {
  const p = await fixture();
  const selected = { checked: true };
  let focused = false;
  const question = { querySelectorAll: () => [selected], querySelector: () => ({ focus() { focused = true; } }) };
  const button = { closest: () => question };
  await p.byId('messages').click({ closest: selector => selector === '[data-question-clear]' ? button : null });
  assert.equal(selected.checked, false);
  assert.equal(focused, true);
  assert.equal(p.calls.length, 0);
});

test('an empty answer does not silently submit a suggested default', async () => {
  const p = await fixture();
  p.entries[0].choices = [];
  await p.submit();
  assert.equal(p.calls.length, 0);
  assert.match(p.status.textContent, /answer each question/);
  await p.byId('messages').dispatchEvent({ type: 'input', target: { closest: () => p.form } });
  assert.equal(p.status.textContent, '');
});

test('network failure preserves the whole draft and permits retry in plain language', async () => {
  const p = await fixture([new TypeError('fetch failed ECONNRESET'), json(204)]);
  await p.submit();
  assert.equal(p.fields.every(field => !field.disabled), true);
  assert.equal(p.entries[1].text, '  Meals for the family  ');
  assert.equal(p.status.textContent, 'Could not send your answers. Please try again.');
  await p.submit();
  assert.equal(p.calls.filter(call => call.method === 'POST').length, 2);
});

test('skipping requires no answers and explicitly reports cancellation', async () => {
  const p = await fixture();
  p.entries.forEach(entry => { entry.choices = []; entry.text = ''; });
  await p.skip();
  assert.deepEqual(JSON.parse(p.calls[0].body), { answers: [], cancelled: true });
});

test('an answered form in another tab is retired and cannot be submitted again', async () => {
  const p = await fixture([json(409)]);
  await p.submit();
  assert.equal(p.fields.every(field => field.disabled), true);
  assert.match(p.status.textContent, /no longer waiting/);
  assert.equal(p.swaps.length, 1);
});

test('ending a session disables any question form still on screen', async () => {
  const p = await fixture();
  await p.event('exhibit_session_closed');
  assert.equal(p.fields.every(field => field.disabled), true);
  assert.equal(p.status.textContent, 'This conversation has ended.');
});
