import { test, expect, login } from '../fixtures';

test('Share opens at the beginning on short screens and text entry waits for a deliberate action', async ({ browser, api, artifacts }) => {
  const artifact = await artifacts.create('<h1>Share opening fixture</h1>');
  const link = await api.post('/api/shares', { data: { artifact_id: artifact.id } });
  expect(link.ok(), await link.text()).toBeTruthy();
  for (const touch of [false, true]) {
    const context = await browser.newContext({ hasTouch: touch, isMobile: touch });
    try {
      await login(context);
      const page = await context.newPage();
      for (const [width, height] of [[320, 640], [390, 640], [640, 360], [1280, 800]]) {
        await page.setViewportSize({ width, height });
        for (const colorScheme of ['light', 'dark'] as const) {
          await page.emulateMedia({ colorScheme });
          await page.goto(`/artifacts/${artifact.id}`);
          await page.evaluate(() => document.fonts.ready);
          const open = page.getByRole('button', { name: 'Share', exact: true });
          await open.click();
          const panel = page.locator('#share-panel');
          const body = panel.locator('.modal-body');
          await expect(panel).toBeVisible();
          expect(await page.evaluate(() => matchMedia('(pointer: coarse)').matches)).toBe(touch);
          expect(await page.evaluate(() => matchMedia('(prefers-color-scheme: dark)').matches)).toBe(colorScheme === 'dark');
          for (let opening = 0; opening < 2; opening++) {
            expect(await body.evaluate(element => element.scrollTop), `${width}x${height} ${colorScheme} touch=${touch} opening=${opening}`).toBe(0);
            expect(await panel.locator('.modal-header').evaluate(header => header.contains(document.activeElement))).toBe(true);
            expect(await panel.locator('.share-data-note').evaluate(note => note.getBoundingClientRect().top >= note.closest('.modal-body')!.getBoundingClientRect().top)).toBe(true);
            const input = panel.locator('#share-add-input');
            await input.click();
            await expect(input).toBeFocused();
            await body.evaluate(element => { element.scrollTop = element.scrollHeight; });
            await panel.locator('#share-close').click();
            await expect(open).toBeFocused();
            if (opening === 0) await open.click();
          }
        }
      }
    } finally {
      await context.close();
    }
  }
});
