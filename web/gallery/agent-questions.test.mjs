import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const script = fileURLToPath(new URL('../../internal/api/assets/gallery/agent-questions-controller.js', import.meta.url));
const json = (status, body = {}) => ({ ok: status < 400, status, json: async () => body });

async function fixture({ responses = [json(204)], count = 1, multiSelect = false, getResponse = () => json(200), beforeSwap = async () => {} } = {}) {
  const calls = [], swaps = [], cards = new Map(), fields = [];
  const status = { textContent: '' }, retry = { hidden: true }, progress = { textContent: '' };
  const skipTrigger = { hidden: false, focus() { this.focused = true; } };
  const confirmation = { hidden: true, querySelector: () => skipTrigger };
  const card = { dataset: { sessionId: 'session-1', interactionId: 'request-1', interactionKind: 'questions', pending: 'true', revision: '1' },
    setAttribute() {}, removeAttribute() {}, scrollIntoView() {},
    querySelector: selector => selector === '.interaction-status' ? status : retry,
    querySelectorAll: () => fields };
  const form = { dataset: {}, setAttribute() {}, closest: selector => selector === '[data-question-form]' ? form : card,
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
    question.setAttribute = (name, value) => { question[name] = value; };
    question.removeAttribute = name => { delete question[name]; };
    question.querySelector = selector => selector === 'textarea' ? text : selector === 'details' ? details : choices[0];
    question.querySelectorAll = selector => selector.includes('aria-pressed') ? choices.filter(choice => choice.pressed === 'true') : choices;
    Object.assign(question, { text, details, choices, send });
    fields.push(text, send, ...choices);
    return question;
  });
  fields.push(retry);
  const apiFetch = async (path, options = {}) => {
    calls.push({ path, ...options });
    if (options.method === 'GET') return getResponse();
    const next = responses.shift();
    if (next instanceof Error) throw next;
    return typeof next === 'function' ? next() : next || json(204);
  };
  const page = loadPageScript([], { apiFetch, AbortController,
    CustomEvent: class { constructor(type, options) { this.type = type; Object.assign(this, options); } },
    htmx: { async ajax(verb, path, options) {
      swaps.push({ verb, path, ...options });
      await options.fetch(path, { method: verb });
      await beforeSwap();
      let cancelled = false;
      await page.context.document.dispatchEvent({ type: 'htmx:before:swap', detail: { ctx: options }, preventDefault() { cancelled = true; } });
      if (!cancelled) cards.set('agent-interaction-request-1', card);
    } }
  });
  cards.set('agent-interaction-request-1', card);
  const byId = page.context.document.getElementById;
  page.context.document.getElementById = id => id.startsWith('agent-interaction-') ? cards.get(id) || null : byId(id);
  page.byId('messages').querySelectorAll = () => [card];
  const { default: QuestionsController } = await page.loadModule(script);
  const controller = new QuestionsController({ scope: { element: form } });
  Object.assign(controller, { statusTarget: status, progressTarget: progress, skipConfirmationTarget: confirmation, skipTriggerTarget: skipTrigger });
  controller.connect();
  await page.context.document.dispatchEvent({ type: 'exhibit:agent-session-changed', detail: { sessionId: 'session-1' } });
  const event = (type, extra = {}) => page.context.document.dispatchEvent({
    type: 'exhibit:agent-event', detail: { sessionId: 'session-1', event: { type, requestId: 'request-1', ...extra } }
  });
  const click = target => controller.choose({ currentTarget: target });
  const choose = (index = 0, option = 0) => click(questions[index].choices[option]);
  const enter = (index = 0, extra = {}) => controller.keyDown({ target: questions[index].text, key: 'Enter', preventDefault() {}, ...extra });
  const submit = () => controller.send({ preventDefault() {} });
  const type = async (value, index = 0) => {
    questions[index].text.value = value;
    controller.edit({ target: questions[index].text });
  };
  const reply = async (message, attachments = {}) => {
    const detail = { sessionId: 'session-1', requestId: 'request-1', message, ...attachments };
    await page.context.document.dispatchEvent({ type: 'exhibit:question-message', detail });
    return detail.response;
  };
  const skip = () => controller.skip();
  return { ...page, calls, swaps, cards, fields, status, retry, questions, form, event, click, choose, enter, type, reply, skip, submit, controller, progress, confirmation };
}

test('choices stay editable and only explicit Send answers submits the batch', async () => {
  const p = await fixture({ count: 2 });
  await p.choose(0);
  await p.type('Plan meals', 1);
  assert.equal(p.calls.length, 0);
  assert.equal(p.progress.textContent, '2 of 2 answered');
  await p.choose(0, 1);
  await p.submit();
  assert.deepEqual(JSON.parse(p.calls[0].body), { revision: 1, action: 'answer', answers: [
    { id: 'q0', selected: ['List'], text: '' }, { id: 'q1', selected: [], text: 'Plan meals' }
  ] });
  assert.equal(p.fields.every(field => field.disabled), true);
  assert.equal(p.swaps[0].swap, 'outerHTML');
});

test('plain Enter and IME never send; Ctrl+Enter explicitly sends a complete set', async () => {
  const p = await fixture();
  await p.type('  A monthly calendar  ');
  await p.enter();
  await p.enter(0, { shiftKey: true });
  await p.enter(0, { ctrlKey: true, isComposing: true });
  assert.equal(p.calls.length, 0);
  await p.enter(0, { ctrlKey: true });
  assert.equal(JSON.parse(p.calls[0].body).answers[0].text, 'A monthly calendar');
});

test('selecting an option replaces a custom single answer, and typing replaces the option', async () => {
  const p = await fixture();
  await p.type('A monthly calendar');
  p.questions[0].details.open = true;
  await p.choose();
  assert.equal(p.questions[0].text.value, '');
  assert.equal(p.questions[0].details.open, false);
  await p.type('A daily plan');
  assert.equal(p.questions[0].choices[0].pressed, 'false');
  assert.equal(p.calls.length, 0);
});

test('multiple selections and optional details are sent together', async () => {
  const p = await fixture({ multiSelect: true });
  await p.choose(0, 0);
  await p.choose(0, 1);
  await p.type('Show both together');
  assert.equal(p.calls.length, 0);
  await p.submit();
  assert.deepEqual(JSON.parse(p.calls[0].body).answers[0], { id: 'q0', selected: ['Weekly', 'List'], text: 'Show both together' });
});

test('incomplete batches focus the missing question without submitting a default', async () => {
  const p = await fixture({ count: 2 });
  await p.choose(0);
  await p.submit();
  assert.equal(p.calls.length, 0);
  assert.equal(p.questions[1].choices[0].focused, true);
  assert.match(p.status.textContent, /Answer each question/);
});

test('skip protects drafted answers and Keep answering preserves the draft', async () => {
  const p = await fixture();
  await p.choose();
  await p.skip();
  assert.equal(p.calls.length, 0);
  assert.equal(p.confirmation.hidden, false);
  p.controller.keepAnswering();
  assert.equal(p.confirmation.hidden, true);
  assert.equal(p.questions[0].choices[0].pressed, 'true');
  await p.skip();
  await p.controller.confirmSkip();
  assert.deepEqual(JSON.parse(p.calls[0].body), { action: 'skip', revision: 1 });
});

test('chat can redirect a whole batch without inventing answers', async () => {
  const p = await fixture({ count: 2 });
  assert.equal(await p.reply('Make a shopping list instead.'), true);
  assert.deepEqual(JSON.parse(p.calls[0].body), { action: 'message', revision: 1, message: 'Make a shopping list instead.' });
});

test('chat forwards captured images and selections with the reply', async () => {
  const p = await fixture();
  const attachments = { images: [{ data: 'base64', mime_type: 'image/png' }], snippets: ['<button>Selected</button>'] };
  assert.equal(await p.reply('Make this larger', attachments), true);
  assert.deepEqual(JSON.parse(p.calls[0].body), { action: 'message', revision: 1, message: 'Make this larger', ...attachments });
});

test('closing or replacing a session aborts loads and ignores late failures', async () => {
  for (const close of [true, false]) {
    let reject;
    const p = await fixture({ getResponse: () => new Promise((_, no) => { reject = no; }) });
    p.cards.clear();
    const loading = p.event('exhibit_interaction', { pending: true, revision: 1 });
    await Promise.resolve();
    if (close) await p.event('exhibit_session_closed');
    else await p.context.document.dispatchEvent({ type: 'exhibit:agent-session-changed', detail: { sessionId: 'session-2' } });
    assert.equal(p.calls[0].signal.aborted, true);
    reject(new Error('connection closed'));
    await loading;
    assert.equal(p.byId('interaction-load-error').hidden, true);
    const requests = p.calls.length;
    await p.byId('interaction-retry').click();
    assert.equal(p.calls.length, requests, 'no retry may target the retired session');
  }
});

test('a retired session cannot swap in a late successful response', async () => {
  let resume, reached;
  const paused = new Promise(resolve => { reached = resolve; });
  const p = await fixture({ beforeSwap: async () => {
    reached();
    await new Promise(resolve => { resume = resolve; });
  } });
  p.cards.clear();
  const loading = p.event('exhibit_interaction', { pending: true, revision: 1 });
  await paused;
  await p.event('exhibit_session_closed');
  resume();
  await loading;
  assert.equal(p.cards.size, 0);
  assert.equal(p.byId('interaction-load-error').hidden, true);
});

test('a delayed answer error cannot update a retired question form', async () => {
  let finish, reached;
  const reading = new Promise(resolve => { reached = resolve; });
  const p = await fixture({ responses: [{ ok: false, status: 409, json: async () => {
    reached();
    return new Promise(resolve => { finish = resolve; });
  } }] });
  const sending = p.reply('Make a shopping list instead');
  await reading;
  await p.event('exhibit_session_closed');
  finish({ error: 'Already answered' });
  assert.equal(await sending, false);
  assert.equal(p.status.textContent, 'This conversation has ended.');
  assert.equal(p.calls.length, 1);
});

test('a failed send keeps the draft and can be retried using Send answers', async () => {
  const p = await fixture({ responses: [new TypeError('fetch failed ECONNRESET'), json(204)] });
  await p.choose();
  await p.submit();
  assert.equal(p.fields.every(field => !field.disabled), true);
  assert.equal(p.questions[0].choices[0].pressed, 'true');
  
  assert.equal(p.status.textContent, 'Could not send. Please try again.');
  await p.submit();
  assert.equal(p.calls.filter(call => call.method === 'POST').length, 2);
});

test('a racing chat reply and choice cannot send the tool two responses', async () => {
  let finish;
  const p = await fixture({ responses: [() => new Promise(resolve => { finish = resolve; })] });
  await p.choose();
  const choice = p.submit();
  assert.equal(await p.reply('A different direction'), false);
  finish(json(204));
  await choice;
  assert.equal(p.calls.filter(call => call.method === 'POST').length, 1);
});

test('reconnect preserves drafts until a completed response is fetched', async () => {
  const p = await fixture();
  p.cards.clear();
  await p.event('exhibit_interaction', { pending: true, revision: 1 });
  assert.equal(p.swaps[0].swap, 'beforeend');
  await p.type('Unfinished');
  await p.event('exhibit_interaction', { pending: true, revision: 1 });
  assert.equal(p.swaps.length, 1);
  assert.equal(p.questions[0].text.value, 'Unfinished');
  await p.event('exhibit_interaction', { pending: false, revision: 2 });
  assert.equal(p.swaps[1].swap, 'outerHTML');
});

test('skip reports cancellation without requiring an answer', async () => {
  const p = await fixture();
  await p.skip();
  assert.deepEqual(JSON.parse(p.calls[0].body), { action: 'skip', revision: 1 });
});

test('another tab answering retires this form without submitting again', async () => {
  const p = await fixture({ responses: [json(409)] });
  await p.choose();
  await p.submit();
  assert.equal(p.fields.every(field => field.disabled), true);
  assert.match(p.status.textContent, /no longer waiting/);
  assert.equal(p.swaps.length, 1);
});

test('a failed send cannot re-enable a form after the session ends', async () => {
  let reject;
  const p = await fixture({ responses: [() => new Promise((_, no) => { reject = no; })] });
  await p.choose();
  const sending = p.submit();
  await p.event('exhibit_session_closed');
  reject(new Error('connection closed'));
  await sending;
  assert.equal(p.fields.every(field => field.disabled), true);
});
