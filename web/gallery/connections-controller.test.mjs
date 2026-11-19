import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript } from './testdom.mjs';

const script = fileURLToPath(new URL('../../internal/api/assets/gallery/connections-controller.js', import.meta.url));

async function setup(responses = []) {
  const calls = [], approvals = [], confirmations = [], changes = [];
  let confirm = true;
  const page = loadPageScript([], {
    CustomEvent, Headers,
    FormData: class {
      constructor(fields) { this.fields = fields.formDataFields || fields; }
      [Symbol.iterator]() { return Object.entries(this.fields)[Symbol.iterator](); }
      get(name) { return this.fields[name] ?? null; }
    },
    fetch: async (path, options = {}) => {
      calls.push({ path, method: options.method || 'GET', data: options.body ? JSON.parse(options.body) : null });
      const response = responses.shift();
      if (response) return response;
      if (path === '/api/connections') return Response.json({ connections: [{ id: 'selected-connection', origins: ['https://accounts.example.test'] }] });
      return options.method === 'DELETE' ? new Response(null, { status: 204 }) : Response.json({}, { status: 201 });
    },
    apiFetch: async (path, options) => { approvals.push({ path, data: JSON.parse(options.body) }); return Response.json({}); },
    confirm: message => { confirmations.push(message); return confirm; },
  });
  const { default: Connections } = await page.loadModule(script);
  page.document.body.addEventListener('exhibit:connections-changed', () => changes.push('connections-changed'));
  const controller = Object.create(Connections.prototype);
  controller.artifactIdValue = 'connected-app';
  controller.feedbackTarget = page.byId('feedback');
  let pending;
  controller.action = (_event, work) => { pending = work(); return pending; };
  return {
    ...page, calls, approvals, confirmations, changes, controller,
    setConfirmation(value) { confirm = value; },
    async grant(fields) { controller.grant({ currentTarget: fields }); await pending; },
    async clear() { controller.clearData({}); await pending; },
    async reconnect(dataset) { controller.reconnect({ currentTarget: { dataset } }); await pending; },
    async repair(id, fields) { controller.repair({ currentTarget: { dataset: { connectionId: id }, formDataFields: fields, reset() {} } }); await pending; },
  };
}

function mismatchResponse(origin = 'https://api-openai.co') {
  return Response.json({
    error: 'this looks like an Anthropic API key, which normally goes to https://api.anthropic.com, but it would be sent to ' + origin,
    code: 'credential_origin_mismatch',
    credential: 'Anthropic API key',
    expected_origin: 'https://api.anthropic.com',
    origin,
  }, { status: 409 });
}

function fieldSection(recipes, ...inputs) {
  return { dataset: { recipes }, hidden: false, querySelectorAll: () => inputs };
}

// The create form is one form with a recipe select. This drives the shipped
// controller over the same fields a browser submits, including disabled ones,
// so a stale value can be caught where it would actually be read.
async function setupCreate(responses = []) {
  const calls = [], finished = [];
  const queue = [...responses];
  let currentAction;
  const page = loadPageScript([], {
    CustomEvent, Headers,
    FormData: class {
      constructor(form) { this.fields = form.formDataFields || form; }
      [Symbol.iterator]() { return Object.entries(this.fields)[Symbol.iterator](); }
      get(name) { return this.fields[name] ?? null; }
    },
    fetch: async (path, options = {}) => {
      calls.push({ path, method: options.method || 'GET', data: options.body ? JSON.parse(options.body) : null });
      const response = queue.shift();
      if (typeof response === 'function') return response();
      return response || Response.json({ id: 'new-connection' }, { status: 201 });
    },
    confirm: () => true,
  });
  const { default: Connections } = await page.loadModule(script);
  const provider = page.byId('connection-kind');
  const elements = {
    name: page.byId('connection-name'),
    origin: page.byId('connection-origin'),
    secret: page.byId('connection-secret'),
    access_url: page.byId('connection-access-url'),
    username: page.byId('connection-user'),
    password: page.byId('connection-password'),
    daily_limit: page.byId('connection-daily-limit'),
    cache_minutes: page.byId('connection-cache-minutes'),
    header: page.byId('connection-header'),
  };
  const createFormTarget = {
    elements,
    get formDataFields() {
      return {
        recipe: provider.value,
        ...Object.fromEntries(Object.entries(elements).map(([name, element]) => [name, element.value])),
      };
    },
  };
  const controller = Object.create(Connections.prototype);
  Object.assign(controller, {
    artifactIdValue: 'connected-app',
    returnUrlValue: '/connections',
    providerTarget: provider,
    fieldsTargets: [],
    createFormTarget,
    createErrorTarget: page.byId('create-error'),
    createButtonTarget: page.byId('create-button'),
    createWarningTarget: page.byId('create-warning'),
    createWarningTextTarget: page.byId('create-warning-text'),
    confirmButtonTarget: page.byId('confirm-button'),
    destinationTarget: page.byId('destination'),
    destinationHostTarget: page.byId('destination-host'),
    feedbackTarget: page.byId('feedback'),
    hasCreateWarningTarget: true,
    hasDestinationTarget: true,
  });
  controller.action = (_event, work) => { currentAction = work(); return currentAction; };
  controller.setupFinished = id => finished.push(id);
  const submitEvent = () => ({ preventDefault() {}, submitter: page.byId('create-button') });
  return {
    ...page, controller, calls, finished, provider, elements, createFormTarget,
    async create() { await controller.create(submitEvent()); },
    async confirm() { controller.confirmCreate(submitEvent()); await currentAction; },
  };
}

test('granting a connector sends only personal credential access', async () => {
  const page = await setup();
  await page.grant({ connection: 'selected-connection', sharing: 'on', egress: 'https://legacy.example.test' });
  assert.deepEqual(page.calls, [{
    path: '/api/artifacts/connected-app/connections', method: 'POST',
    data: { connection_id: 'selected-connection' },
  }]);
  assert.deepEqual(page.approvals, []);
  assert.equal(page.controller.feedbackTarget.textContent, 'Connection access granted.');
});

test('optional origin approval adds only the selected connector API without replacing other approvals', async () => {
  const page = await setup();
  await page.grant({ connection: 'selected-connection', approve_origins: 'on' });
  assert.deepEqual(page.approvals, [{
    path: '/api/artifacts/connected-app/origins',
    data: { origin: 'https://accounts.example.test', decision: 'allow', source: 'connection-consent' },
  }]);
  assert.deepEqual(page.calls, [{ path: '/api/connections', method: 'GET', data: null }, {
    path: '/api/artifacts/connected-app/connections', method: 'POST',
    data: { connection_id: 'selected-connection' },
  }]);
});

test('saved-data cleanup is optional and does not change connection grants or origin approvals', async () => {
  const page = await setup();
  page.setConfirmation(false);
  await page.clear();
  assert.deepEqual(page.calls, []);
  page.setConfirmation(true);
  await page.clear();
  assert.deepEqual(page.calls, [{ path: '/api/artifacts/connected-app/connection-data', method: 'DELETE', data: null }]);
  assert.deepEqual(page.approvals, []);
  assert.match(page.confirmations[0], /Account and network permissions stay unchanged/);
  assert.equal(page.controller.feedbackTarget.textContent, 'Saved data cleared.');
});


test('OAuth reconnect carries the existing account and originating tool', async () => {
  const page = await setup([Response.json({url:'https://provider.example/authorize'})]);
  const navigations = [];
  page.window.location.assign = url => navigations.push(url);
  await page.reconnect({connectionId:'existing-account',connectionKind:'oauth',provider:'github',connectionName:'My GitHub'});
  assert.deepEqual(page.calls, [{path:'/api/connections/oauth/github/begin',method:'POST',data:{name:'My GitHub',artifact_id:'connected-app',connection_id:'existing-account'}}]);
  assert.deepEqual(navigations,['https://provider.example/authorize']);
});

test('native reconnect replaces credentials on the same account instead of creating another', async () => {
  const page = await setup([Response.json({id:'existing-account'})]);
  const finished = [];
  page.controller.setupFinished = id => finished.push(id);
  await page.repair('existing-account',{secret:'replacement-secret'});
  assert.deepEqual(page.calls,[{path:'/api/connections/existing-account/reconnect',method:'POST',data:{secret:'replacement-secret'}}]);
  assert.deepEqual(finished,['existing-account']);
});

test('failed repair stays on setup and does not recreate dependent access', async () => {
  const page = await setup([Response.json({error:'The credential could not be replaced.'},{status:503})]);
  let finished = false;
  page.controller.setupFinished = () => { finished = true; };
  await assert.rejects(page.repair('existing-account',{secret:'replacement-secret'}),/could not be replaced/);
  assert.equal(finished,false);
  assert.equal(page.calls.length,1);
});

test('the destination follows the active recipe instead of a disabled Basic URL', async () => {
  const page = await setupCreate();
  page.controller.fieldsTargets = [
    fieldSection('api-key bearer basic', page.elements.origin),
    fieldSection('api-key bearer', page.elements.secret),
    fieldSection('basic', page.elements.access_url, page.elements.username, page.elements.password),
    fieldSection('api-key bearer basic', page.elements.daily_limit, page.elements.cache_minutes),
  ];

  page.provider.value = 'basic';
  page.controller.providerChanged();
  page.elements.access_url.value = 'https://basic-user:basic-pass@basic.example.test/base';
  page.controller.basicURLChanged();
  assert.equal(page.controller.destinationHostTarget.textContent, 'basic.example.test');
  assert.equal(page.elements.origin.disabled, true);

  page.provider.value = 'bearer';
  page.controller.providerChanged();
  assert.equal(page.elements.access_url.disabled, true);
  assert.equal(page.controller.destinationTarget.hidden, true, 'a disabled Basic credential URL is not the destination');
  assert.equal(page.controller.destinationHostTarget.textContent, '');

  page.elements.origin.value = 'https://api-openai.co';
  page.controller.destinationChanged();
  assert.equal(page.controller.destinationTarget.hidden, false);
  assert.equal(page.controller.destinationHostTarget.textContent, 'api-openai.co');
});

test('editing the form after a mismatch warning requires a fresh submit', async () => {
  const page = await setupCreate([mismatchResponse(), mismatchResponse()]);
  page.provider.value = 'bearer';
  page.elements.name.value = 'Anthropic account';
  page.elements.origin.value = 'https://api-openai.co';
  page.elements.secret.value = 'sk-ant-api03-not-a-real-key';

  await page.create();
  assert.equal(page.controller.createWarningTarget.hidden, false);
  assert.match(page.controller.createWarningTextTarget.textContent, /api-openai\.co/);
  assert.equal(page.calls.length, 1);
  assert.equal(page.calls[0].data.confirm_origin_mismatch, undefined);

  page.controller.formEdited();
  assert.equal(page.controller.createWarningTarget.hidden, true);
  assert.equal(page.controller.pendingCreate, null);
  await page.confirm();
  assert.equal(page.calls.length, 1, 'a stale confirmation cannot post');

  await page.create();
  assert.equal(page.calls.length, 2);
  assert.equal(page.calls[1].data.confirm_origin_mismatch, undefined, 'the fresh submit is unconfirmed');
  assert.equal(page.controller.createWarningTarget.hidden, false);
  assert.match(page.controller.createWarningTextTarget.textContent, /api-openai\.co/);

  await page.confirm();
  assert.equal(page.calls.length, 3);
  assert.equal(page.calls[2].data.confirm_origin_mismatch, true);
  assert.deepEqual(page.finished, ['new-connection']);
});

test('a mismatch answer that arrives after an edit cannot restore the warning', async () => {
  let release;
  const page = await setupCreate([() => new Promise(resolve => {
    release = () => resolve(mismatchResponse());
  })]);
  page.provider.value = 'bearer';
  page.elements.name.value = 'Anthropic account';
  page.elements.origin.value = 'https://api-openai.co';
  page.elements.secret.value = 'sk-ant-api03-not-a-real-key';

  const creating = page.create();
  assert.equal(page.calls.length, 1);
  page.controller.formEdited();
  release();
  await creating;

  assert.equal(page.controller.createWarningTarget.hidden, true);
  assert.equal(page.controller.pendingCreate, null);
  await page.confirm();
  assert.equal(page.calls.length, 1, 'the obsolete answer never lights a confirm button');
});
