import { test, expect } from '../fixtures';

test('sticky editor Save names its action and shows pending and persisted outcomes', async ({ page, api, artifacts }) => {
  const artifact = await artifacts.create('<h1>Save feedback fixture</h1>');
  for (const width of [1280, 390, 320]) {
    await page.setViewportSize({ width, height: 844 });
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.emulateMedia({ colorScheme });
      await page.goto(`/artifacts/${artifact.id}/edit`);
      const save = page.locator('#save-button');
      const status = page.locator('.app-header [role=status]');
      await expect(status).toHaveText('Saved');
      await expect(save).toBeDisabled();
      const title = `Saved ${width} ${colorScheme}`;
      await page.getByLabel('Artifact title', { exact: true }).fill(title);
      await expect(status).toHaveText('Unsaved');
      await expect(save).toBeEnabled();
      await page.locator('#tags-panel > summary').click();
      await page.locator('#versions-panel > summary').click();
      await page.locator('#versions-panel').evaluate(element => { element.scrollIntoView({ block: 'start' }); window.scrollBy(0, -72); });
      const visibleLabel = await save.locator('.header-link-label').evaluate(element => {
        const rect = element.getBoundingClientRect();
        return rect.width > 20 && rect.height > 10 && rect.y >= 0 && rect.bottom <= 56;
      });
      expect(visibleLabel, `${width}px ${colorScheme} visible Save label at a deep section`).toBe(true);
      let release!: () => void;
      const held = new Promise<void>(resolve => { release = resolve; });
      let writes = 0;
      await page.route(`**/api/artifacts/${artifact.id}`, async route => {
        if (route.request().method() !== 'PATCH') return route.continue();
        writes++;
        await held;
        await route.continue();
      });
      await save.click();
      await expect(status).toHaveText('Saving…');
      await expect(save).toBeDisabled();
      await expect(save).toHaveAttribute('aria-busy', 'true');
      await page.screenshot({ path: test.info().outputPath(`saving-${width}-${colorScheme}.png`) });
      await save.evaluate(button => (button as HTMLButtonElement).click());
      release();
      await expect(status).toHaveText('Saved');
      await expect(save).toBeDisabled();
      expect(writes).toBe(1);
      expect(new URL(page.url()).pathname).toBe(`/artifacts/${artifact.id}/edit`);
      expect((await (await api.get(`/api/artifacts/${artifact.id}`)).json()).title).toBe(title);
      await page.screenshot({ path: test.info().outputPath(`saved-${width}-${colorScheme}.png`) });
      await page.unroute(`**/api/artifacts/${artifact.id}`);
    }
  }
});

test('failed editor Save keeps its explanation visible and supports a successful retry', async ({ page, api, artifacts }) => {
  const artifact = await artifacts.create('<h1>Save retry fixture</h1>', 'Original title');
  await page.setViewportSize({ width: 320, height: 844 });
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme });
    await page.goto(`/artifacts/${artifact.id}/edit`);
    await page.getByLabel('Artifact title', { exact: true }).fill(`Retry ${colorScheme}`);
    await page.locator('#versions-panel > summary').click();
    await page.locator('#versions-panel').evaluate(element => { element.scrollIntoView({ block: 'start' }); window.scrollBy(0, -72); });
    const before = (await (await api.get(`/api/artifacts/${artifact.id}`)).json()).title;
    await page.route(`**/api/artifacts/${artifact.id}`, route => route.request().method() === 'PATCH'
      ? route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Temporary save failure. Try again.' }) })
      : route.continue());
    const save = page.locator('#save-button');
    await save.click();
    await expect(page.locator('.app-header [role=status]')).toHaveText('Review');
    const explanation = page.locator('.editor-save-message');
    await expect(explanation).toBeVisible();
    await expect(explanation).toContainText('Temporary save failure');
    expect(await explanation.evaluate(element => { const r = element.getBoundingClientRect(); return r.top >= 0 && r.bottom <= innerHeight && r.left >= 0 && r.right <= innerWidth; })).toBe(true);
    await expect(save).toBeEnabled();
    expect((await (await api.get(`/api/artifacts/${artifact.id}`)).json()).title).toBe(before);
    await page.screenshot({ path: test.info().outputPath(`save-error-320-${colorScheme}.png`) });
    await page.unroute(`**/api/artifacts/${artifact.id}`);
    await save.click();
    await expect(page.locator('.app-header [role=status]')).toHaveText('Saved');
    await expect(explanation).toBeHidden();
    expect((await (await api.get(`/api/artifacts/${artifact.id}`)).json()).title).toBe(`Retry ${colorScheme}`);
  }
});
