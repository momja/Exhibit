import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { loadPageScript, recordingApi } from './testdom.mjs';

const assets = new URL('../../internal/api/assets/gallery/', import.meta.url);
const asset = (name) => fileURLToPath(new URL(name, assets));
const storage = () => {
  const values = new Map();
  return { values, getItem: (key) => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
};

async function load({ artifact = 'art-1', viewer = '1', store = storage(), blocked = false, readOnly = false, connectionProtected = false } = {}) {
  const tabs = [];
  const api = recordingApi();
  const openURL = `/artifacts/${artifact}/open`;
  const page = loadPageScript(['network-prompt.js', 'state-api.js', 'detail.js'].map(asset), {
    ID: artifact, OPEN_URL: openURL, SOURCE_URL: '', TOKEN: '', READ_ONLY: readOnly,
    CONNECTION_PROTECTED: connectionProtected,
    STATE_WRITABLE: !readOnly, SHARED_STATE: false,
    downloadsApproved: false, clipboardApproved: false, linksApproved: false,
    cameraApproved: false, microphoneApproved: false, geolocationApproved: false,
    apiFetch: api.apiFetch, apiStateFetch: api.apiStateFetch, CustomEvent,
    localStorage: store,
    open: (...args) => {
      const tab = { args, opener: 'library', location: null, closed: false, close() { this.closed = true; } };
      tabs.push(tab);
      return blocked ? null : tab;
    }
  }, Object.fromEntries([
    'net-modal', 'dl-modal', 'clip-modal', 'link-modal', 'media-modal', 'geo-modal',
    'capability-warning-banner', 'origin-blocked-banner', 'direct-open-modal'
  ].map((id) => [id, { hidden: true }])));
  page.byId('direct-open-modal').dataset = { artifact, viewer, openUrl: openURL };
  await page.loadModule(asset('direct-open.js'));
  const trigger = page.byId('direct-open-toolbar');
  const prompt = page.byId('direct-open-modal');
  const checkbox = page.byId('direct-open-remember');
  const confirm = page.byId('direct-open-confirm');
  const cancel = page.byId('direct-open-cancel');
  return { ...page, api, tabs, trigger, prompt, checkbox, confirm, cancel, store, openURL, directURL: openURL + '?direct=1' };
}

test('both toolbar and capability banner ask before opening exactly one isolated tab', async () => {
  const page = await load();
  for (const trigger of [page.trigger, page.byId('direct-open-banner')]) {
    const before = page.tabs.length;
    await trigger.click();
    assert.equal(page.prompt.hidden, false);
    assert.equal(page.document.activeElement, page.cancel);
    assert.equal(page.checkbox.checked, false);
    assert.equal(page.tabs.length, before);
    await page.confirm.click();
    assert.equal(page.tabs.length, before + 1);
    assert.deepEqual(page.tabs.at(-1).args, ['', '_blank']);
    assert.equal(page.tabs.at(-1).opener, null);
    assert.equal(page.tabs.at(-1).location, page.directURL);
    assert.equal(page.prompt.hidden, true);
  }
  assert.equal(page.store.values.size, 0, 'unchecked confirmation re-prompts');
  assert.deepEqual(page.api.calls, [], 'opening is not an artifact mutation');
});

for (const dismiss of ['cancel', 'escape', 'backdrop']) {
  test(`${dismiss} opens nothing, resets the checkbox and restores focus`, async () => {
    const page = await load();
    await page.trigger.click();
    page.checkbox.checked = true;
    if (dismiss === 'cancel') await page.cancel.click();
    if (dismiss === 'escape') page.pressKey('Escape');
    if (dismiss === 'backdrop') await page.prompt.click();
    assert.equal(page.prompt.hidden, true);
    assert.equal(page.document.activeElement, page.trigger);
    assert.equal(page.checkbox.checked, false);
    assert.equal(page.tabs.length, 0);
    assert.equal(page.store.values.size, 0);
    await page.trigger.click();
    assert.equal(page.prompt.hidden, false);
    assert.equal(page.checkbox.checked, false);
  });
}

test('dialog content clicks do not dismiss; keyboard focus wraps inside the modal', async () => {
  const page = await load();
  await page.trigger.click();
  await page.prompt.click(page.checkbox);
  assert.equal(page.prompt.hidden, false);
  page.confirm.focus();
  page.pressKey('Tab');
  assert.equal(page.document.activeElement, page.checkbox);
  page.pressKey('Tab', { shiftKey: true });
  assert.equal(page.document.activeElement, page.confirm);
});

test('a hidden toolbar menu returns focus to More actions', async () => {
  const page = await load();
  await page.trigger.click();
  page.trigger.closest = () => page.byId('pip-menu');
  const lookup = page.document.getElementById;
  page.document.getElementById = (id) => id === 'pv-open' ? null : lookup(id);
  await page.cancel.click();
  assert.equal(page.document.activeElement, page.byId('pip-more'));
});

test('background controls are inert while the modal is open and restored on dismissal', async () => {
  const page = await load();
  const alreadyInert = { inert: true };
  page.document.body.children = [page.prompt, page.trigger, alreadyInert];
  await page.trigger.click();
  assert.equal(page.trigger.inert, true);
  assert.equal(page.prompt.inert, undefined);
  await page.cancel.click();
  assert.equal(page.trigger.inert, false);
  assert.equal(alreadyInert.inert, true);
});

test('dismissal during an approval closes the placeholder and never opens or remembers later', async () => {
  const page = await load();
  let approve;
  let result;
  const approval = new Promise((resolve) => { approve = resolve; });
  page.document.dispatchEvent(new CustomEvent('exhibit:open-directly', {
    detail: { approve: () => approval, resolve: (opened) => { result = opened; } }
  }));
  page.checkbox.checked = true;
  const confirming = page.confirm.click();
  await page.confirm.click();
  assert.equal(page.tabs.length, 1, 'double click cannot claim another tab');
  await page.cancel.click();
  assert.equal(result, false);
  assert.equal(page.tabs[0].closed, true);
  approve(true);
  await confirming;
  assert.equal(page.tabs[0].location, null);
  assert.equal(page.store.values.size, 0);
});

test('only checked confirmation persists, including for read-only recipients, across reloads', async () => {
  const store = storage();
  const first = await load({ store, readOnly: true });
  await first.trigger.click();
  first.checkbox.checked = true;
  assert.equal(store.values.size, 0, 'checking alone does not remember');
  await first.confirm.click();
  assert.equal(store.values.size, 1);
  assert.deepEqual(first.api.calls, []);
  for (const page of [first, await load({ store, readOnly: true })]) {
    await page.trigger.click();
    assert.equal(page.prompt.hidden, true);
    assert.equal(page.tabs.at(-1).location, page.directURL);
  }
  for (const options of [{ artifact: 'art-2' }, { viewer: '2' }, { viewer: 'anonymous' }]) {
    const other = await load({ store, ...options });
    await other.trigger.click();
    assert.equal(other.prompt.hidden, false, 'another artifact/viewer still asks');
    assert.equal(other.tabs.length, 0);
  }
});

for (const failure of ['getItem', 'setItem']) {
  test(`storage ${failure} failure still opens and asks again on reload`, async () => {
    const store = storage();
    store[failure] = () => { throw new Error('storage unavailable'); };
    const page = await load({ store });
    await page.trigger.click();
    page.checkbox.checked = true;
    await page.confirm.click();
    assert.equal(page.tabs.length, 1);
    assert.equal(page.tabs[0].location, page.directURL);
    const reload = await load({ store });
    await reload.trigger.click();
    assert.equal(reload.prompt.hidden, false);
  });
}

test('blocked popups leave a retryable dialog, make one attempt, and do not remember', async () => {
  const page = await load({ blocked: true });
  await page.trigger.click();
  page.checkbox.checked = true;
  await page.confirm.click();
  assert.equal(page.tabs.length, 1);
  assert.equal(page.prompt.hidden, false);
  assert.match(page.byId('direct-open-status').textContent, /blocked/);
  assert.equal(page.confirm.disabled, false);
  assert.equal(page.store.values.size, 0);
  await page.cancel.click();
});

for (const [kind, request] of [
  ['media', { __avMedia: true, id: 'm1', video: true, audio: true }],
  ['geo', { __avGeolocation: true, id: 1 }]
]) {
  test(`${kind} approval shares the gate; Escape returns to its original decision`, async () => {
    const page = await load();
    await page.postFromFrame({ ...request, artifactId: 'art-1' });
    page.byId(`${kind}-allow`).focus();
    const allowing = page.byId(`${kind}-allow`).click();
    assert.equal(page.prompt.hidden, false);
    assert.equal(page.tabs.length, 0);
    assert.deepEqual(page.api.calls, []);
    page.pressKey('Escape');
    await allowing;
    assert.equal(page.byId(`${kind}-modal`).hidden, false);
    assert.equal(page.document.activeElement, page.byId(`${kind}-allow`));
    assert.equal(page.tabs.length, 0);
    assert.deepEqual(page.api.calls, []);
    const retrying = page.byId(`${kind}-allow`).click();
    await page.confirm.click();
    await retrying;
    assert.equal(page.tabs.length, 1);
    assert.equal(page.tabs[0].location, page.directURL);
    assert.equal(page.byId(`${kind}-modal`).hidden, true);
  });
}

test('frame reloads bypass the gate and never create a tab', async () => {
  const page = await load();
  await page.byId('state-changed-reload').click();
  assert.equal(page.prompt.hidden, true);
  assert.equal(page.tabs.length, 0);
  assert.ok(page.frame.src.startsWith(page.openURL + '?r='));
});

// A delegated click models a newly rendered agent control, without reloading
// the module. The dialog remains in the page while the controls are swapped.
async function clickAgentOpen(page, artifact, { disabled = false } = {}) {
  const trigger = page.byId('agent-open-' + (page.agentButtonCount = (page.agentButtonCount || 0) + 1));
  trigger.dataset = { directOpenArtifact: artifact, directOpenUrl: `/artifacts/${artifact}/open` };
  trigger.disabled = disabled;
  const icon = { closest: (selector) => selector === '[data-direct-open-artifact]' ? trigger : null };
  await page.document.dispatchEvent({ type: 'click', target: icon });
  return trigger;
}

test('agent opens after creation and swaps use current artifact and share detail preferences', async () => {
  const store = storage();
  const page = await load({ artifact: '', store });
  const trigger = await clickAgentOpen(page, 'created');
  assert.equal(page.prompt.hidden, false);
  await page.cancel.click();
  assert.equal(page.document.activeElement, trigger);
  assert.equal(page.tabs.length, 0);
  await clickAgentOpen(page, 'created');
  page.checkbox.checked = true;
  await page.confirm.click();
  assert.equal(page.tabs.at(-1).location, '/artifacts/created/open?direct=1');
  assert.equal(page.tabs.at(-1).opener, null);
  await clickAgentOpen(page, 'created');
  assert.equal(page.prompt.hidden, true, 'replacement button uses remembered choice');
  const detail = await load({ artifact: 'created', store });
  await detail.trigger.click();
  assert.equal(detail.prompt.hidden, true, 'detail shares the agent preference');
  await clickAgentOpen(page, 'different');
  assert.equal(page.prompt.hidden, false, 'different artifact still asks');
  page.checkbox.checked = true;
  // Even if the preview changes while asking, confirm the artifact named by
  // the clicked control and never remember its choice against the next one.
  page.prompt.dataset.artifact = 'next';
  await page.confirm.click();
  assert.equal(page.tabs.at(-1).location, '/artifacts/different/open?direct=1');
  await clickAgentOpen(page, 'next');
  assert.equal(page.prompt.hidden, false);
});

test('agent uses a choice remembered from detail and restores focus after a preview swap', async () => {
  const store = storage();
  const detail = await load({ store });
  await detail.trigger.click();
  detail.checkbox.checked = true;
  await detail.confirm.click();
  const agent = await load({ artifact: '', store });
  await clickAgentOpen(agent, 'art-1');
  assert.equal(agent.prompt.hidden, true);
  assert.equal(agent.tabs.at(-1).location, detail.directURL);
  const old = await clickAgentOpen(agent, 'other');
  old.isConnected = false;
  await agent.cancel.click();
  assert.equal(agent.document.activeElement, agent.byId('pv-open'));
});

test('direct opens preserve existing query parameters and replace the intent marker', async () => {
  const page = await load();
  page.prompt.dataset.openUrl = page.openURL + '?view=history&direct=0#saved';
  await page.trigger.click();
  await page.confirm.click();
  assert.equal(page.tabs[0].location, page.openURL + '?view=history&direct=1#saved');
});

test('protected detail controls and capability events cannot spend a remembered direct-open choice', async () => {
  const store = storage();
  const earlier = await load({ store });
  await earlier.trigger.click();
  earlier.checkbox.checked = true;
  await earlier.confirm.click();
  const page = await load({ store, connectionProtected: true });
  await page.trigger.click();
  let result;
  let approvals = 0;
  await page.document.dispatchEvent(new CustomEvent('exhibit:open-directly', {
    detail: { approve: () => { approvals++; return true; }, resolve: opened => { result = opened; } }
  }));
  assert.equal(result, false);
  assert.equal(approvals, 0);
  assert.equal(page.prompt.hidden, true);
  assert.equal(page.tabs.length, 0);
  assert.deepEqual(page.api.calls, []);
});

test('a newly protected agent control cannot spend the earlier detail preference', async () => {
  const store = storage();
  const detail = await load({ store });
  await detail.trigger.click();
  detail.checkbox.checked = true;
  await detail.confirm.click();
  const agent = await load({ artifact: '', store });
  await clickAgentOpen(agent, 'art-1', { disabled: true });
  assert.equal(agent.prompt.hidden, true);
  assert.equal(agent.tabs.length, 0);
});

for (const [kind, request, resultFlag] of [
  ['media', { __avMedia: true, id: 'm1', video: true }, '__avMediaResult'],
  ['geo', { __avGeolocation: true, id: 1 }, '__avGeolocationResult']
]) {
  test(`protected ${kind} requests are answered without a direct-open or capability prompt`, async () => {
    const page = await load({ connectionProtected: true });
    await page.postFromFrame({ ...request, artifactId: 'art-1' });
    assert.equal(page.byId(`${kind}-modal`).hidden, true);
    assert.equal(page.prompt.hidden, true);
    assert.equal(page.tabs.length, 0);
    assert.deepEqual(page.api.calls, []);
    const answer = page.frame.contentWindow.posted.find(message => message[resultFlag]);
    assert.equal(answer.banner, false);
    assert.match(answer.error, /Connected data must stay inside Exhibit/);
  });
}
