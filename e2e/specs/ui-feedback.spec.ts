import { test, expect } from '../fixtures';

test('creation validation names and focuses the field for each method', async ({ page }) => {
  let submissions = 0;
  page.on('request', request => {
    if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/artifacts') submissions++;
  });
  await page.goto('/new');
  await page.getByRole('button', { name: 'Start building', exact: true }).click();
  const description = page.getByRole('textbox', { name: 'What should it do?', exact: true });
  await expect(description).toBeFocused();
  await expect(description).toHaveAttribute('aria-invalid', 'true');
  await expect(page.locator('#agent-description-error')).toBeVisible();
  await expect(page.locator('#status')).toBeEmpty();

  await page.locator('[data-mode=paste]').click();
  const source = page.getByRole('textbox', { name: 'HTML source', exact: true });
  await page.getByRole('button', { name: 'Add to Gallery', exact: true }).click();
  await expect(source).toBeFocused();
  await expect(source).toHaveAttribute('aria-invalid', 'true');
  await expect(source).toHaveAttribute('aria-describedby', 'body-error');
  await expect(page.locator('#body-error')).toBeVisible();
  await expect(page.locator('#status')).toBeEmpty();

  await page.locator('[data-mode=url]').click();
  const url = page.getByRole('textbox', { name: 'Page URL', exact: true });
  await expect(page.locator('#status')).toBeEmpty();
  await page.getByRole('button', { name: 'Add to Gallery', exact: true }).click();
  await expect(url).toBeFocused();
  await expect(page.locator('#status')).toBeEmpty();
  await url.fill('not a page address');
  await page.getByRole('button', { name: 'Add to Gallery', exact: true }).click();
  await expect(url).toBeFocused();
  await expect(page.locator('#url-input-error')).toHaveText('Enter a full http:// or https:// page URL.');
  await expect(page.locator('#status')).toBeEmpty();
  await expect(page.getByRole('button', { name: 'Add to Gallery', exact: true })).toBeEnabled();
  await expect(page.locator('[data-mode=paste]')).toBeEnabled();
  expect(submissions).toBe(0);
});

test('pending creation submits once, preserves input, and recovers from a refusal', async ({ page }) => {
  let submissions = 0;
  let release!: () => void;
  const responseGate = new Promise<void>(resolve => { release = resolve; });
  await page.route('**/api/artifacts', async route => {
    if (route.request().method() !== 'POST') return route.continue();
    submissions++;
    await responseGate;
    await route.fulfill({ status: 503, contentType: 'text/plain', body: 'Please try again.' });
  });
  await page.goto('/new');
  await page.locator('[data-mode=paste]').click();
  const source = page.getByRole('textbox', { name: 'HTML source', exact: true });
  await source.fill('<h1>Keep my working copy</h1>');
  await source.press('ControlOrMeta+f');
  const search = page.locator('.cm-search');
  await search.locator('input[name=search]').fill('Keep');
  await search.locator('input[name=replace]').fill('Changed');
  await page.getByRole('button', { name: 'Add to Gallery', exact: true }).click();
  await expect(page.locator('#ingest-panel')).toHaveAttribute('aria-busy', 'true');
  await expect(page.getByRole('button', { name: 'Add to Gallery', exact: true })).toBeDisabled();
  await expect(page.locator('[data-mode=url]')).toBeDisabled();
  // Search commands must respect the editor state, not just contentEditable.
  await search.getByRole('button', { name: 'Replace all', exact: true }).click();
  await expect(source).toHaveText('<h1>Keep my working copy</h1>');
  await page.locator('#ingest-panel').evaluate(form => {
    (form as HTMLFormElement).requestSubmit();
    (form as HTMLFormElement).requestSubmit();
  });
  expect(submissions).toBe(1);
  release();
  await expect(page.locator('#status')).toHaveText('Couldn’t add the artifact: Please try again.');
  await expect(page.locator('#ingest-panel')).toHaveAttribute('aria-busy', 'false');
  await expect(page.getByRole('button', { name: 'Add to Gallery', exact: true })).toBeEnabled();
  await expect(page.locator('[data-mode=url]')).toBeEnabled();
  await expect(source).toHaveText('<h1>Keep my working copy</h1>');
  await source.fill('<h1>Retry with an edit</h1>');
});

test('history shares desktop columns and fits a narrow phone with enlarged text', async ({ page, api, artifacts }) => {
  const artifact = await artifacts.create('<h1>Original tool</h1>', 'A long tool name for the history layout');
  for (const body of ['<h1>Second version</h1>', '<h1>Third version</h1>']) {
    const response = await api.patch(`/api/artifacts/${artifact.id}`, { data: { body } });
    expect(response.ok(), await response.text()).toBeTruthy();
  }
  await page.goto(`/artifacts/${artifact.id}/edit`);
  await page.locator('#versions-panel > summary').click();
  await expect(page.locator('.version-row')).toHaveCount(3);
  for (const width of [1280, 768]) {
    await page.setViewportSize({ width, height: 844 });
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.emulateMedia({ colorScheme });
      const columns = await page.locator('#versions-rows').evaluate(list => {
        const rows = [...list.querySelectorAll('.version-row')];
        const positions = (selector: string) => rows.map(row => row.querySelector(selector)!.getBoundingClientRect().left);
        return {
          dates: positions('.version-when'),
          actions: positions('.version-actions'),
          actionPairsFit: rows.every(row => {
            const buttons = [...row.querySelectorAll('.version-actions button')].map(button => button.getBoundingClientRect());
            return buttons.length < 2 || Math.abs(buttons[0].top - buttons[1].top) < 1;
          }),
        };
      });
      expect(Math.max(...columns.dates) - Math.min(...columns.dates), `date alignment ${width}px ${colorScheme}`).toBeLessThan(1);
      expect(Math.max(...columns.actions) - Math.min(...columns.actions), `action alignment ${width}px ${colorScheme}`).toBeLessThan(1);
      expect(columns.actionPairsFit).toBe(true);
    }
  }
  await page.setViewportSize({ width: 320, height: 844 });
  await page.evaluate(() => { document.documentElement.style.fontSize = '24px'; });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await expect(page.getByRole('textbox', { name: 'Artifact source', exact: true })).toBeVisible();
  await page.goto(`/artifacts/${artifact.id}`);
  const more = page.getByRole('button', { name: 'More actions', exact: true });
  await more.focus();
  await more.press('ArrowDown');
  const menu = page.locator('#pip-menu');
  await expect(menu).toBeVisible();
  await page.keyboard.press('End');
  await expect(page.getByRole('button', { name: 'Hide controls', exact: true })).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(more).toBeFocused();
  await expect(menu).toBeHidden();
  await more.click();
  await page.getByRole('button', { name: 'Hide controls', exact: true }).click();
  const restore = page.getByRole('button', { name: 'Show artifact controls', exact: true });
  await expect(restore).toBeVisible();
  const box = await restore.boundingBox();
  expect(Math.round(box!.width)).toBeGreaterThanOrEqual(44);
  await restore.click();
  await expect(page.locator('#pip')).toBeVisible();
});
