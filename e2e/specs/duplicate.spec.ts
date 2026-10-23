import { randomUUID } from 'node:crypto';
import { type Page } from '@playwright/test';
import { test, expect } from '../fixtures';

const isDuplicate = (request: { method(): string; url(): string }) =>
  request.method() === 'POST' && new URL(request.url()).pathname.endsWith('/duplicate');

async function openDuplicate(page: Page, title: string) {
  const trigger = page.getByRole('button', { name: `Actions for ${title}`, exact: true });
  await trigger.focus();
  await trigger.press('ArrowDown');
  await expect(page.getByRole('menu', { name: `Actions for ${title}`, exact: true })).toBeVisible();
  await page.keyboard.press('End');
  await page.keyboard.press('Enter');
  const dialog = page.getByRole('dialog', { name: 'Duplicate artifact', exact: true });
  await expect(dialog).toBeVisible();
  const input = dialog.getByRole('textbox', { name: 'Title', exact: true });
  await expect(input).toBeFocused();
  await expect(input).toHaveValue(`${title} (copy)`);
  return { trigger, dialog, input };
}

test('keyboard focus stays in the modal and every cancellation path restores the menu trigger', async ({ page, artifacts }) => {
  const artifact = await artifacts.create('<h1>Cancellation fixture</h1>');
  const mutations: string[] = [];
  page.on('request', request => { if (isDuplicate(request)) mutations.push(request.url()); });
  await page.goto('/');
  for (const dismiss of ['Escape', 'Cancel', 'backdrop']) {
    const { trigger, dialog, input } = await openDuplicate(page, artifact.title);
    for (const key of ['Tab', 'Shift+Tab']) {
      for (let step = 0; step < 6; step++) {
        await page.keyboard.press(key);
        await expect.poll(() => dialog.evaluate(node => node.contains(document.activeElement))).toBe(true);
      }
    }
    // Clicking the dialog's own padding must preserve the working copy.
    await input.fill('Unsaved working title');
    await dialog.click({ position: { x: 4, y: 4 } });
    await expect(input).toHaveValue('Unsaved working title');
    if (dismiss === 'Escape') await page.keyboard.press('Escape');
    else if (dismiss === 'Cancel') await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    else await page.mouse.click(8, 8);
    await expect(dialog).not.toBeVisible();
    await expect(trigger).toBeFocused();
  }
  expect(mutations).toHaveLength(0);
});

test('save refreshes server-rendered cards and replacement components work across two cycles', async ({ page, artifacts }) => {
  let artifact = await artifacts.create('<h1>Replacement fixture</h1>');
  await page.goto('/');
  const documentRequests: string[] = [];
  const mutations: string[] = [];
  page.on('request', request => {
    if (request.isNavigationRequest() && request.frame() === page.mainFrame()) documentRequests.push(request.url());
    if (isDuplicate(request)) mutations.push(request.url());
  });
  for (let cycle = 0; cycle < 2; cycle++) {
    const { trigger, dialog, input } = await openDuplicate(page, artifact.title);
    const oldTrigger = await trigger.elementHandle();
    const title = `E2E copy ${randomUUID()}`;
    await input.fill(title);
    const saved = page.waitForResponse(response => isDuplicate(response.request()));
    const refreshed = page.waitForResponse(response => new URL(response.url()).pathname === '/partials/gallery-grid');
    await dialog.getByRole('button', { name: 'Duplicate artifact', exact: true }).click();
    const response = await saved;
    expect(response.status()).toBe(201);
    artifact = artifacts.track(await response.json());
    const fragment = await refreshed;
    expect(fragment.ok()).toBeTruthy();
    expect(fragment.request().headers()['hx-request']).toBe('true');
    const link = page.getByRole('link', { name: title, exact: true });
    await expect(link).toBeVisible();
    await expect(link).toBeFocused();
    await expect.poll(() => oldTrigger!.evaluate(node => node.isConnected)).toBe(false);
    await oldTrigger?.dispose();
  }
  // Reopen on the newest nodes too; no stale targets or duplicated feature listeners.
  const { dialog } = await openDuplicate(page, artifact.title);
  await page.keyboard.press('Escape');
  await expect(dialog).not.toBeVisible();
  expect(mutations).toHaveLength(2);
  expect(documentRequests).toHaveLength(0);
});

test('Escape, backdrop, and repeated submission cannot dismiss or duplicate an in-flight save', async ({ page, artifacts }) => {
  const artifact = await artifacts.create('<h1>Pending fixture</h1>');
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  let mutations = 0;
  await page.route('**/api/artifacts/*/duplicate', async route => {
    mutations++;
    await gate;
    await route.continue();
  });
  await page.goto('/');
  const { dialog, input } = await openDuplicate(page, artifact.title);
  try {
    const requested = page.waitForRequest(isDuplicate);
    await dialog.getByRole('button', { name: 'Duplicate artifact', exact: true }).click();
    await requested;
    await expect(input).toBeDisabled();
    await expect(dialog.getByRole('button', { name: 'Cancel', exact: true })).toBeDisabled();
    await page.keyboard.press('Escape');
    await page.mouse.click(8, 8);
    await page.keyboard.press('Enter');
    await expect(dialog).toBeVisible();
    expect(mutations).toBe(1);
    const saved = page.waitForResponse(response => isDuplicate(response.request()));
    release();
    const response = await saved;
    expect(response.status()).toBe(201);
    const copy = artifacts.track(await response.json());
    await expect(dialog).not.toBeVisible();
    await expect(page.getByRole('link', { name: copy.title, exact: true })).toBeFocused();
  } finally {
    release();
  }
});

test('a committed save with a lost response retries the same identity and creates only one copy', async ({ page, artifacts, api }) => {
  const artifact = await artifacts.create('<h1>Uncertain response fixture</h1>');
  const bodies: unknown[] = [];
  let committedID = '';
  await page.route('**/api/artifacts/*/duplicate', async route => {
    bodies.push(route.request().postDataJSON());
    if (bodies.length === 1) {
      const response = await route.fetch();
      expect(response.status()).toBe(201);
      const copy = artifacts.track(await response.json());
      committedID = copy.id;
      await route.fulfill({ status: 503, contentType: 'application/json', body: '{"error":"Lost upstream response"}' });
    } else await route.continue();
  });
  await page.goto('/');
  const { dialog, input } = await openDuplicate(page, artifact.title);
  const title = `E2E uncertain ${randomUUID()}`;
  await input.fill(title);
  await dialog.getByRole('button', { name: 'Duplicate artifact', exact: true }).click();
  await expect(dialog.getByRole('status')).toHaveText('Could not confirm duplication. Try again to check the saved result.');
  await expect(input).toBeDisabled();
  const retried = page.waitForResponse(response => isDuplicate(response.request()));
  await dialog.getByRole('button', { name: 'Try again', exact: true }).click();
  const response = await retried;
  expect(response.status()).toBe(200);
  expect((await response.json()).id).toBe(committedID);
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole('link', { name: title, exact: true })).toBeFocused();
  expect(bodies).toHaveLength(2);
  expect(bodies[1]).toEqual(bodies[0]);
  const matches = await api.get('/api/artifacts', { params: { q: title } });
  expect((await matches.json()).filter((item: { title: string }) => item.title === title)).toHaveLength(1);
});

test('a definite refusal leaves editable input and permits a corrected save', async ({ page, artifacts }) => {
  const artifact = await artifacts.create('<h1>Refusal fixture</h1>');
  const bodies: { request_id: string }[] = [];
  await page.route('**/api/artifacts/*/duplicate', async route => {
    bodies.push(route.request().postDataJSON());
    if (bodies.length === 1) await route.fulfill({ status: 413, contentType: 'application/json', body: '{"error":"Storage quota exceeded"}' });
    else await route.continue();
  });
  await page.goto('/');
  const { dialog, input } = await openDuplicate(page, artifact.title);
  await dialog.getByRole('button', { name: 'Duplicate artifact', exact: true }).click();
  await expect(dialog.getByRole('status')).toHaveText('Storage quota exceeded');
  await expect(input).toBeEditable();
  const title = `E2E corrected ${randomUUID()}`;
  await input.fill(title);
  const saved = page.waitForResponse(response => isDuplicate(response.request()));
  await dialog.getByRole('button', { name: 'Try again', exact: true }).click();
  const response = await saved;
  expect(response.status()).toBe(201);
  artifacts.track(await response.json());
  await expect(page.getByRole('link', { name: title, exact: true })).toBeFocused();
  expect(bodies).toHaveLength(2);
  expect(bodies[0].request_id).not.toBe(bodies[1].request_id);
});
