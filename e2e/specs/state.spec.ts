import { readFileSync } from 'node:fs';
import { test, expect, login } from '../fixtures';
import { appOrigin, renderOrigin } from '../environment';

const counterHTML = readFileSync(new URL('../fixtures/counter.html', import.meta.url), 'utf8');

test('localStorage survives reload and a fresh owner browser context through server state', async ({ page, browser, api, artifacts }) => {
  const artifact = await artifacts.create(counterHTML);
  const render = page.waitForResponse(response => response.url().startsWith(`${renderOrigin}/a/${artifact.id}?`));
  await page.goto(`/artifacts/${artifact.id}`);
  expect((await render).headers()['cache-control']).toBe('no-store');
  const iframe = page.locator('iframe').first();
  await expect(iframe).toHaveAttribute('sandbox', 'allow-scripts allow-forms');
  expect(new URL(await iframe.getAttribute('src') as string).origin).toBe(renderOrigin);
  expect(appOrigin).not.toBe(renderOrigin);
  const tool = iframe.contentFrame();
  await expect(tool.getByLabel('Startup counter')).toHaveText('0');
  await tool.getByRole('button', { name: 'Increment', exact: true }).click();
  await tool.getByRole('button', { name: 'Increment', exact: true }).click();
  await expect(tool.getByLabel('Current counter')).toHaveText('2');
  await expect.poll(async () => {
    const response = await api.get(`/api/artifacts/${artifact.id}/state`);
    expect(response.ok()).toBeTruthy();
    return response.json();
  }).toEqual({ counter: '2' });

  await page.reload();
  await expect(tool.getByLabel('Startup counter')).toHaveText('2');
  // No copied storageState or context-level headers: the API is the only
  // source available to a second device's synchronous artifact startup read.
  const other = await browser.newContext();
  try {
    await login(other);
    const otherPage = await other.newPage();
    await otherPage.goto(`${appOrigin}/artifacts/${artifact.id}`);
    await expect(otherPage.locator('iframe').first().contentFrame().getByLabel('Startup counter')).toHaveText('2');
  } finally {
    await other.close();
  }
});
