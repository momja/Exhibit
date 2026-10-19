import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const script = fileURLToPath(new URL('../../internal/api/assets/gallery/agent-questions.js', import.meta.url));
const json = (status, body = {}) => ({ ok: status < 400, status, json: async () => body });

async function fixture({ responses = [json(204)], count = 1, multiSelect = false } = {}) {
  const calls = [], swaps = [], cards = new Map(), fields = [];
  const status = { textContent: '' }, retry = { hidden: true };
  const card = { dataset: { sessionId: 'session-1', requestId: 'request-1', pending: 'true' },
    querySelector: selector => selector === '.question-status' ? status : retry,
    querySelectorAll: () => fields };
  const form = { dataset: {}, closest: selector => selector === '[data-question-form]' ? form : card,
    querySelector: () => status, querySelectorAll: selector => selector === 'input, textarea, button' ? fields : questions };
  const questions = Array.from({ length: count }, (_, index) => {
    const question = { dataset: { questionId: 'q' + index, multiSelect: String(multiSelect) }, closest: () => form };
    const text = { value: '', tagName: 'TEXTAREA', focus() { this.focused = true; },
      closest: selector => selector === '[data-question-form]' ? form : question };
    const details = { open: false };
    const choices = ['Weekly', 'List'].map(value => ({ value, pressed: 'false',
      setAttribute(name, value) { this.pressed = value; }, getAttribute() { return this.pressed; },
      focus() { this.focused = true; },
      closest: selector => selector === '[data-question-choice]' ? choices.find(c => c.value === value) : selector === '[data-question-id]' ? question : null }));
    const send = { closest: selector => selector === '[data-question-answer]' ? send : selector === '[data-question-id]' ? question : null };
    question.querySelector = selector => selector === 'textarea' ? text : selector === 'details' ? details : choices[0];
    question.querySelectorAll = selector => selector.includes('aria-pressed') ? choices.filter(choice => choice.pressed === 'true') : choices;
    Object.assign(question, { text, details, choices, send });
    fields.push(text, send, ...choices);
    return question;
  });
  fields.push(retry);
  const apiFetch = async (path, options = {}) => {
    calls.push({ path, ...options });
    if (options.method === 'GET') return json(200);
    const next = responses.shift();
    if (next instanceof Error) throw next;
    return typeof next === 'function' ? next() : next || json(204);
  };
  const page = loadPageScript([], { apiFetch,
    CustomEvent: class { constructor(type, options) { this.type = type; Object.assign(this, options); } },
    htmx: { async ajax(verb, path, options) {
      swaps.push({ verb, path, ...options });
      await options.fetch(path, { method: verb });
      cards.set('agent-question-request-1', card);
    } }
  });
  cards.set('agent-question-request-1', card);
  const byId = page.context.document.getElementById;
  page.context.document.getElementById = id => id.startsWith('agent-question-') ? cards.get(id) || null : byId(id);
  page.byId('messages').querySelectorAll = () => [form];
  await page.loadModule(script);
  const event = (type, extra = {}) => page.context.document.dispatchEvent({
    type: 'exhibit:agent-event', detail: { sessionId: 'session-1', event: { type, requestId: 'request-1', ...extra } }
  });
  const click = target => page.byId('messages').click(target);
  const choose = (index = 0, option = 0) => click(questions[index].choices[option]);
  const enter = (index = 0, extra = {}) => page.byId('messages').dispatchEvent({
    type: 'keydown', target: questions[index].text, key: 'Enter', preventDefault() {}, ...extra
  });
  const type = async (value, index = 0) => {
    questions[index].text.value = value;
    await page.byId('messages').dispatchEvent({ type: 'input', target: questions[index].text });
  };
  const reply = async message => {
    const detail = { sessionId: 'session-1', requestId: 'request-1', message };
    await page.context.document.dispatchEvent({ type: 'exhibit:question-message', detail });
    return detail.response;
  };
  const skip = () => click({ closest: selector => selector === '[data-question-skip]' ? { closest: () => form } : null });
  return { ...page, calls, swaps, cards, fields, status, retry, questions, form, event, click, choose, enter, type, reply, skip };
}

test('a single choice sends immediately, without a separate submit action', async () => {
  const p = await fixture();
  await p.choose();
  assert.deepEqual(JSON.parse(p.calls[0].body), { answers: [{ id: 'q0', selected: ['Weekly'], text: '' }], cancelled: false });
  assert.equal(p.fields.every(field => field.disabled), true);
  assert.equal(p.swaps[0].swap, 'outerHTML');
});

test('stacked questions wait for the last answer and keep each earlier choice', async () => {
  const p = await fixture({ count: 2 });
  await p.choose(0);
  assert.equal(p.calls.length, 0);
  assert.equal(p.questions[1].choices[0].focused, true);
  await p.choose(0, 1);
  await p.choose(1);
  assert.deepEqual(JSON.parse(p.calls[0].body).answers.map(answer => answer.selected), [['List'], ['Weekly']]);
});

test('Something else waits for Enter, preserving newlines and IME composition', async () => {
  const p = await fixture();
  await p.type('  A monthly calendar  ');
  await p.enter(0, { shiftKey: true });
  await p.enter(0, { isComposing: true });
  assert.equal(p.calls.length, 0);
  await p.enter();
  assert.equal(JSON.parse(p.calls[0].body).answers[0].text, 'A monthly calendar');
});

test('the send icon commits a written answer; a later choice does not send a draft', async () => {
  const p = await fixture({ count: 2 });
  await p.type('Help with meals', 1);
  await p.choose(0);
  assert.equal(p.calls.length, 0);
  await p.click(p.questions[1].send);
  assert.equal(JSON.parse(p.calls[0].body).answers[1].text, 'Help with meals');
});

test('choosing a suggestion clears the custom answer and closes Something else', async () => {
  const p = await fixture();
  await p.type('A monthly calendar');
  p.questions[0].details.open = true;
  await p.choose();
  assert.equal(p.questions[0].text.value, '');
  assert.equal(p.questions[0].details.open, false);
  assert.equal(JSON.parse(p.calls[0].body).answers[0].text, '');
});

test('editing an earlier answer makes it a draft again until explicitly sent', async () => {
  const p = await fixture({ count: 2 });
  await p.choose(0);
  await p.type('A monthly calendar');
  assert.equal(p.questions[0].choices[0].pressed, 'false');
  await p.choose(1);
  assert.equal(p.calls.length, 0);
  await p.enter(0);
  assert.equal(JSON.parse(p.calls[0].body).answers[0].text, 'A monthly calendar');
});

test('multiple selections can be combined before Done sends them', async () => {
  const p = await fixture({ multiSelect: true });
  await p.choose(0, 0);
  await p.choose(0, 1);
  await p.type('Show both together');
  assert.equal(p.calls.length, 0);
  await p.click(p.questions[0].send);
  assert.deepEqual(JSON.parse(p.calls[0].body).answers[0], { id: 'q0', selected: ['Weekly', 'List'], text: 'Show both together' });
});

test('an empty written answer does not submit a default', async () => {
  const p = await fixture();
  await p.enter();
  assert.equal(p.calls.length, 0);
  assert.equal(p.questions[0].text.focused, true);
  assert.match(p.status.textContent, /Choose an option or write an answer/);
});

test('chat can redirect a whole batch without inventing answers', async () => {
  const p = await fixture({ count: 2 });
  assert.equal(await p.reply('Make a shopping list instead.'), true);
  assert.deepEqual(JSON.parse(p.calls[0].body), { message: 'Make a shopping list instead.' });
});

test('a failed immediate answer keeps the draft and offers an explicit retry', async () => {
  const p = await fixture({ responses: [new TypeError('fetch failed ECONNRESET'), json(204)] });
  await p.choose();
  assert.equal(p.fields.every(field => !field.disabled), true);
  assert.equal(p.questions[0].choices[0].pressed, 'true');
  assert.equal(p.retry.hidden, false);
  assert.equal(p.status.textContent, 'Could not send your answers. Please try again.');
  await p.byId('messages').dispatchEvent({ type: 'submit', target: p.form, preventDefault() {} });
  assert.equal(p.calls.filter(call => call.method === 'POST').length, 2);
});

test('a racing chat reply and choice cannot send the tool two responses', async () => {
  let finish;
  const p = await fixture({ responses: [() => new Promise(resolve => { finish = resolve; })] });
  const choice = p.choose();
  assert.equal(await p.reply('A different direction'), false);
  finish(json(204));
  await choice;
  assert.equal(p.calls.filter(call => call.method === 'POST').length, 1);
});

test('reconnect preserves drafts until a completed response is fetched', async () => {
  const p = await fixture();
  p.cards.clear();
  await p.event('exhibit_questions', { pending: true });
  assert.equal(p.swaps[0].swap, 'beforeend');
  await p.type('Unfinished');
  await p.event('exhibit_questions', { pending: true });
  assert.equal(p.swaps.length, 1);
  assert.equal(p.questions[0].text.value, 'Unfinished');
  await p.event('exhibit_questions', { pending: false });
  assert.equal(p.swaps[1].swap, 'outerHTML');
});

test('skip reports cancellation without requiring an answer', async () => {
  const p = await fixture();
  await p.skip();
  assert.deepEqual(JSON.parse(p.calls[0].body), { answers: [], cancelled: true });
});

test('another tab answering retires this form without submitting again', async () => {
  const p = await fixture({ responses: [json(409)] });
  await p.choose();
  assert.equal(p.fields.every(field => field.disabled), true);
  assert.match(p.status.textContent, /no longer waiting/);
  assert.equal(p.swaps.length, 1);
});

test('a failed send cannot re-enable a form after the session ends', async () => {
  let reject;
  const p = await fixture({ responses: [() => new Promise((_, no) => { reject = no; })] });
  const sending = p.choose();
  await p.event('exhibit_session_closed');
  reject(new Error('connection closed'));
  await sending;
  assert.equal(p.fields.every(field => field.disabled), true);
});
