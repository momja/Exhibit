import { test, expect } from '../fixtures';

test('Back from a completed import restores a usable creation page', async ({ page, artifacts }) => {
  await page.goto('/new');
  await page.locator('[data-mode=paste]').click();
  await page.getByRole('textbox', { name: 'HTML source', exact: true }).fill('<h1>History import</h1>');
  await page.getByRole('button', { name: 'Add to Gallery', exact: true }).click();
  await expect(page).toHaveURL(/\/artifacts\/[^/]+$/);
  artifacts.track({ id: new URL(page.url()).pathname.split('/').at(-1)!, title: 'History import' });
  await page.goBack();
  await expect(page).toHaveURL(/\/new$/);
  await expect(page.locator('[data-mode=paste]')).toBeEnabled();
  await page.locator('[data-mode=paste]').click();
  await expect(page.getByRole('button', { name: 'Add to Gallery', exact: true })).toBeEnabled();
});

test('persisted-page recovery resets a completed import and preserves unfinished approval', async ({ page, artifacts }) => {
  const artifact = await artifacts.create('<h1>Restoration fixture</h1>');
  for (const complete of [false, true]) {
    // Supply deterministic server outcomes without external network access.
    await page.route('**/api/artifacts', route => route.fulfill({ json: {
      artifact,
      network_footprint: complete ? [] : ['https://example.test'],
      snapshot: complete ? { applied: false, error: 'A snapshot needs review.' } : undefined,
    } }));
    await page.goto('/new');
    await page.locator('[data-mode=paste]').click();
    await page.getByRole('textbox', { name: 'HTML source', exact: true }).fill('<h1>Restoration fixture</h1>');
    await page.getByRole('button', { name: 'Add to Gallery', exact: true }).click();
    await expect(page.locator('#status')).toContainText(complete ? 'Saved —' : 'Review network access');
    await expect(page.locator('[data-mode=paste]')).toBeDisabled();
    if (complete) {
      const reload = page.waitForRequest(request => request.isNavigationRequest() && new URL(request.url()).pathname === '/new');
      await page.evaluate(() => dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true })));
      await reload;
      await expect(page.locator('[data-mode=paste]')).toBeEnabled();
    } else {
      // Exercise the persisted event explicitly; headless Chromium may decline BFCache.
      await page.evaluate(async () => {
        dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true }));
        await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      });
      await expect(page.locator('#status')).toContainText('Review network access');
      await expect(page.locator('[data-mode=paste]')).toBeDisabled();
    }
    await page.unroute('**/api/artifacts');
  }
});
