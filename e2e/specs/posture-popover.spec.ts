import { test, expect } from '../fixtures';

test('posture receipt stays within the viewport through keyboard, resize and server-rendered replacement', async ({ page, api, artifacts }) => {
  const artifact = await artifacts.create('<h1>Posture fixture</h1>');
  const shared = await api.post('/api/shares', { data: { artifact_id: artifact.id } });
  expect(shared.ok(), await shared.text()).toBeTruthy();
  let cardHeader: Record<string, string> | undefined;
  const card = page.locator(`[data-href="/artifacts/${artifact.id}"]`);
  const trigger = card.locator('[data-capability-trigger]');
  const panel = card.locator('.capability-popover');
  for (const width of [1280, 768, 390, 320]) {
    await page.setViewportSize({ width, height: width > 767 ? 500 : 740 });
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.emulateMedia({ colorScheme });
      await page.goto('/');
      await page.evaluate(() => document.fonts.ready);
      const originalHeight = await page.evaluate(() => document.documentElement.scrollHeight);
      await trigger.focus();
      await expect(panel).not.toBeVisible();
      await trigger.press('Enter');
      await expect(panel).toBeVisible();
      const layout = await panel.evaluate(element => {
        const bounds = element.getBoundingClientRect(), style = getComputedStyle(element);
        return { left: bounds.left, right: bounds.right, top: bounds.top, bottom: bounds.bottom,
          fontSize: parseFloat(style.fontSize), lineHeight: parseFloat(style.lineHeight),
          documentHeight: document.documentElement.scrollHeight, viewportWidth: innerWidth, viewportHeight: innerHeight };
      });
      expect(layout.left).toBeGreaterThanOrEqual(15);
      expect(layout.right).toBeLessThanOrEqual(layout.viewportWidth - 15);
      expect(layout.top).toBeGreaterThanOrEqual(15);
      expect(layout.bottom).toBeLessThanOrEqual(layout.viewportHeight - 15);
      expect(layout.documentHeight).toBe(originalHeight);
      expect(layout.fontSize).toBeGreaterThanOrEqual(width > 767 ? 13 : 14);
      expect(layout.lineHeight / layout.fontSize).toBeGreaterThanOrEqual(1.45);
      cardHeader = await panel.locator('.capability-popover-header').first().evaluate(element => {
        const style = getComputedStyle(element);
        return { fontSize: style.fontSize, fontWeight: style.fontWeight, textTransform: style.textTransform };
      });
      await page.setViewportSize({ width, height: width > 767 ? 540 : 700 });
      await expect.poll(() => panel.evaluate(element => element.getBoundingClientRect().bottom <= innerHeight - 15)).toBe(true);
      await trigger.press('Escape');
      await expect(panel).not.toBeVisible();
      await trigger.click();
      await page.mouse.click(width - 2, 2);
      await expect(panel).not.toBeVisible();
      expect(new URL(page.url()).pathname).toBe('/');
    }
  }
  // htmx replaces the card, so the old controller and its window hooks leave.
  const previous = await trigger.elementHandle();
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.locator('header [data-gallery-query]').fill('not-a-fixture');
  await expect(card).toHaveCount(0);
  await expect.poll(() => previous!.evaluate(node => node.isConnected)).toBe(false);
  await page.locator('[data-gallery-clear]').click();
  await expect(card).toBeVisible();
  await trigger.click();
  await expect(panel).toBeVisible();
  // A Safari null-relatedTarget focusout must not swallow the Manage tap.
  await trigger.dispatchEvent('focusout', { relatedTarget: null });
  await expect(panel).toBeVisible();
  await panel.getByRole('link', { name: 'Manage security settings →', exact: true }).click();
  await expect.poll(() => { const url = new URL(page.url()); return url.pathname + url.hash; }).toBe(`/artifacts/${artifact.id}/edit#security-panel`);
  await page.goto(`/artifacts/${artifact.id}`);
  await page.locator('.pip-title').click();
  const viewerHeader = await page.locator('#pip-info .capability-popover-header').first().evaluate(element => {
    const style = getComputedStyle(element);
    return { fontSize: style.fontSize, fontWeight: style.fontWeight, textTransform: style.textTransform };
  });
  expect(viewerHeader).toEqual(cardHeader);
});
