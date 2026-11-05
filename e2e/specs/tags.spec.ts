import { test, expect } from '../fixtures';

test('tag editing keeps color samples intact, controls aligned and validation local in both schemes', async ({ page, api, artifacts }) => {
  test.setTimeout(90_000);
  const artifact = await artifacts.create('<h1>Tag UI fixture</h1>');
  const tags: { id: string; name: string }[] = [];
  try {
    const names = ['Health', 'Reading', 'A long descriptive tag name for narrow layouts', 'Utilities'];
    const colors = ['#377a64', '#8a5db0', '#111111', '#ffffff'];
    for (let i = 0; i < names.length; i++) {
      const response = await api.post('/api/tags', { data: { name: `${names[i]}-${artifact.id.slice(0, 8)}`, color: colors[i] } });
      expect(response.status(), await response.text()).toBe(201);
      tags.push(await response.json());
      expect((await api.post(`/api/tags/${tags[i].id}/artifacts/${artifact.id}`)).ok()).toBeTruthy();
    }
    let writes = 0, nativeDialogs = 0;
    page.on('request', request => { if (request.url().includes('/api/tags') && ['POST', 'PATCH', 'DELETE'].includes(request.method())) writes++; });
    page.on('dialog', dialog => { nativeDialogs++; void dialog.dismiss(); });
    await page.goto(`/artifacts/${artifact.id}/edit`);
    await page.locator('#tags-panel > summary').click();
    await expect(page.locator('#tag-add-select')).toHaveCount(0);
    await expect(page.locator('.tag-available-note')).toHaveText('All existing tags are already on this artifact.');
    for (const width of [1280, 768, 390, 320]) {
      await page.setViewportSize({ width, height: 844 });
      for (const colorScheme of ['light', 'dark'] as const) {
        await page.emulateMedia({ colorScheme });
        await page.evaluate(() => document.fonts.ready);
        expect(await page.evaluate(() => matchMedia('(prefers-color-scheme: dark)').matches)).toBe(colorScheme === 'dark');
        await page.mouse.move(0, 0);
        const rows = await page.locator('.tag-edit-item').evaluateAll(items => items.map(item => {
          const chip = item.querySelector('.tag-pill')!.getBoundingClientRect(), actions = item.querySelector('.tag-edit-actions')!.getBoundingClientRect();
          return { actionsRight: actions.right, separated: chip.right + 7 <= actions.left, overflow: item.scrollWidth - item.clientWidth };
        }));
        expect(rows.every(row => row.separated && row.overflow <= 1)).toBe(true);
        if (width < 768) expect(Math.max(...rows.map(row => row.actionsRight)) - Math.min(...rows.map(row => row.actionsRight))).toBeLessThan(1);
        expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
        const edit = page.getByRole('button', { name: `Edit tag ${tags[0].name}`, exact: true });
        await edit.click();
        const modal = page.locator('#tag-edit-modal');
        await expect(modal).toBeVisible();
        await expect(modal.locator('#tag-edit-scope')).toContainText('every artifact in your library');
        const save = modal.locator('#tag-edit-save');
        await expect(save).toHaveText('Save tag');
        await expect(save).toBeDisabled();
        const cleanTop = await modal.evaluate(element => element.getBoundingClientRect().top);
        const green = modal.getByRole('radio', { name: /^Green / });
        await expect(green).toBeChecked();
        await green.press('ArrowRight');
        await expect(modal.getByRole('radio', { name: /^Teal / })).toBeChecked();
        await expect(modal.locator('#tag-edit-color-hex')).toHaveValue('#42838c');
        await expect(modal.locator('.color-selection')).toHaveText('Teal');
        const hex = modal.locator('#tag-edit-color-hex');
        await hex.fill('#bad!');
        await expect(hex).toHaveAttribute('aria-invalid', 'true');
        await expect(modal.locator('#tag-edit-color-error')).toBeVisible();
        await expect(save).toBeDisabled();
        const invalidPaint = await hex.evaluate(element => {
          const reference = document.createElement('span'); reference.style.color = 'var(--color-danger)'; document.body.append(reference);
          const danger = getComputedStyle(reference).color; reference.remove();
          const field = element.getBoundingClientRect(), error = document.getElementById('tag-edit-color-error')!.getBoundingClientRect();
          return { border: getComputedStyle(element).borderColor, danger, gap: error.top - field.bottom };
        });
        expect(invalidPaint.border, 'invalid hex uses the same danger border as invalid Name').toBe(invalidPaint.danger);
        expect(invalidPaint.gap).toBeGreaterThanOrEqual(4);
        expect(invalidPaint.gap).toBeLessThanOrEqual(8);
        await expect(modal.locator('#tag-edit-color-hint')).toBeHidden();
        await expect(modal.locator('.color-selection')).toBeHidden();
        await expect(modal.locator('.color-empty')).toBeVisible();
        expect(Math.abs(await modal.evaluate(element => element.getBoundingClientRect().top) - cleanTop), 'local color validation does not recenter the sheet').toBeLessThan(1);
        await hex.fill('#8a5db0');
        await expect(modal.locator('.color-selection')).toHaveText('Custom');
        expect(await modal.locator('.color-custom-row').evaluate(element => getComputedStyle(element).outlineStyle)).toBe('none');
        const name = modal.locator('#tag-edit-name');
        await name.fill('');
        await expect(name).toHaveAttribute('aria-invalid', 'true');
        await expect(modal.locator('#tag-edit-name-error')).toHaveText('Name is required.');
        expect(Math.abs(await modal.evaluate(element => element.getBoundingClientRect().top) - cleanTop), 'local name validation does not recenter the sheet').toBeLessThan(1);
        expect(await name.evaluate(element => {
          const field = element.getBoundingClientRect(), error = document.getElementById('tag-edit-name-error')!.getBoundingClientRect();
          return error.top >= field.bottom && error.top - field.bottom <= 8;
        })).toBe(true);
        await name.fill(`${tags[0].name} draft`);
        await expect(save).toBeEnabled();
        await modal.locator('#tag-edit-cancel').focus();
        await page.keyboard.press('Tab');
        await expect(save).toBeFocused();
        expect(await modal.locator('#tag-edit-cancel').evaluate(button => button.getBoundingClientRect().right <= document.getElementById('tag-edit-save')!.getBoundingClientRect().left)).toBe(true);
        await page.screenshot({ path: test.info().outputPath(`tag-edit-${width}-${colorScheme}.png`) });
        await modal.locator('#tag-edit-delete').click();
        await expect(modal.locator('h2')).toHaveText('Delete tag?');
        await expect(modal.locator('#tag-delete-cancel')).toBeFocused();
        await expect(modal.locator('#tag-delete-help')).toContainText('Your artifacts will be kept.');
        await modal.locator('#tag-delete-cancel').click();
        await expect(name).toHaveValue(`${tags[0].name} draft`);
        await modal.locator('#tag-edit-cancel').click();
        await expect(edit).toBeFocused();
      }
    }
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.emulateMedia({ colorScheme });
      await page.locator('#tag-add-name').fill('Local draft');
      await page.locator('#tag-add-color-hex').fill('#12');
      await expect(page.locator('#tag-create-confirm')).toBeDisabled();
      await expect(page.locator('#tag-add-color-hint')).toBeHidden();
      await expect(page.locator('#tag-add-color-controls .color-empty')).toBeVisible();
      await expect(page.locator('#tag-add-color-controls .color-selection')).toBeHidden();
      await page.locator('#tag-add-color-hex').fill('#6b7280');
      await expect(page.locator('#tag-create-confirm')).toBeEnabled();
      await page.locator('#tag-add-name').fill('');
      await expect(page.locator('#tag-create-confirm')).toBeDisabled();
    }
    for (const size of [{ width: 320, height: 640 }, { width: 640, height: 360 }]) {
      await page.setViewportSize(size);
      for (const colorScheme of ['light', 'dark'] as const) {
        await page.emulateMedia({ colorScheme });
        await page.getByRole('button', { name: `Edit tag ${tags[0].name}`, exact: true }).click();
        const modal = page.locator('#tag-edit-modal');
        await modal.locator('.modal-body').hover();
        await page.mouse.wheel(0, 1000);
        await expect(modal.locator('#tag-edit-delete')).toBeVisible();
        expect(await modal.locator('.modal-footer').evaluate(element => { const r = element.getBoundingClientRect(); return r.top >= 0 && r.bottom <= innerHeight; })).toBe(true);
        await modal.locator('#tag-edit-cancel').click();
      }
    }
    expect(writes).toBe(0);
    expect(nativeDialogs).toBe(0);
  } finally {
    for (const tag of tags) expect((await api.delete(`/api/tags/${tag.id}`)).ok()).toBeTruthy();
  }
});

test('partial tag creation retains nearby attach error and retries the same tag', async ({ page, api, artifacts }) => {
  test.setTimeout(60_000);
  const remaining = new Set<string>();
  try {
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.setViewportSize({ width: 390, height: 844 });
      await page.emulateMedia({ colorScheme });
      const artifact = await artifacts.create('<h1>Partial tag creation</h1>');
      const name = `Partial-${artifact.id.slice(0, 8)}`;
      await page.goto(`/artifacts/${artifact.id}/edit`);
      await page.locator('#tags-panel > summary').click();
      let refuse = true, creates = 0;
      page.on('request', request => { if (request.url().endsWith('/api/tags') && request.method() === 'POST') creates++; });
      const attachURL = `**/api/tags/*/artifacts/${artifact.id}`;
      await page.route(attachURL, route => refuse ? route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Temporary attach failure. Try again.' }) }) : route.continue());
      await page.locator('#tag-add-name').fill(name);
      const response = page.waitForResponse(response => response.url().endsWith('/api/tags') && response.request().method() === 'POST');
      await page.locator('#tag-create-confirm').click();
      const created = await (await response).json(); remaining.add(created.id);
      const select = page.locator('#tag-add-select');
      await expect(select).toHaveValue(created.id);
      await expect(select).toBeFocused();
      await expect(page.locator('#tag-add-confirm')).toBeEnabled();
      await expect(page.locator('#tag-add-error')).toHaveText('Tag created, but not added. Temporary attach failure. Try again.');
      expect(await page.locator('#tag-add-error').evaluate(element => {
        const error = element.getBoundingClientRect(), row = document.querySelector('.tag-add-row')!.getBoundingClientRect(), create = document.getElementById('tag-create-heading')!.getBoundingClientRect();
        return error.top >= row.bottom && error.top - row.bottom <= 12 && error.bottom < create.top;
      })).toBe(true);
      await page.screenshot({ path: test.info().outputPath(`tag-partial-create-error-${colorScheme}.png`) });
      refuse = false;
      await page.locator('#tag-add-confirm').click();
      await expect(page.locator('#tag-add-error')).toBeHidden();
      await expect(page.locator(`[data-action=edit-tag][data-tag-id="${created.id}"]`)).toBeFocused();
      expect((await (await api.get(`/api/artifacts/${artifact.id}`)).json()).tags).toHaveLength(1);
      expect(creates).toBe(1);
      await page.unroute(attachURL);
      expect((await api.delete(`/api/tags/${created.id}`)).ok()).toBeTruthy(); remaining.delete(created.id);
    }
  } finally { for (const id of remaining) await api.delete(`/api/tags/${id}`); }
});

test('tag changes persist with clear pending, local failure and retry feedback without page Save', async ({ page, api, artifacts }) => {
  test.setTimeout(90_000);
  const remaining = new Set<string>();
  try {
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.setViewportSize({ width: 390, height: 844 });
      await page.emulateMedia({ colorScheme });
      const first = await artifacts.create('<h1>Tag source</h1>'), second = await artifacts.create('<h1>Tag destination</h1>');
      const original = `Workflow-${first.id.slice(0, 8)}`;
      await page.goto(`/artifacts/${first.id}/edit`);
      await page.locator('#tags-panel > summary').click();
      await page.locator('#tag-add-name').fill(original);
      const createdResponse = page.waitForResponse(response => response.url().endsWith('/api/tags') && response.request().method() === 'POST');
      await page.locator('#tag-create-confirm').click();
      const created = await (await createdResponse).json();
      remaining.add(created.id);
      await expect(page.locator(`[data-action=edit-tag][data-tag-id="${created.id}"]`)).toBeVisible();
      await expect(page.locator('#tag-status')).toHaveText('Tag created and added.');
      await page.goto(`/artifacts/${second.id}/edit`);
      await page.locator('#tags-panel > summary').click();
      await page.locator('#tag-add-select').selectOption(created.id);
      let releaseAdd!: () => void;
      const heldAdd = new Promise<void>(resolve => { releaseAdd = resolve; });
      const attachURL = `**/api/tags/${created.id}/artifacts/${second.id}`;
      await page.route(attachURL, async route => { await heldAdd; await route.continue(); });
      await page.locator('#tag-add-confirm').click();
      await expect(page.locator('#tag-status')).toHaveText('Adding tag…');
      await expect(page.locator('#tags-panel')).toHaveAttribute('aria-busy', 'true');
      await page.screenshot({ path: test.info().outputPath(`tag-adding-${colorScheme}.png`) });
      releaseAdd();
      await expect(page.locator('#tag-status')).toHaveText('Tag added.');
      await expect(page.locator(`[data-action=edit-tag][data-tag-id="${created.id}"]`)).toBeVisible();
      await page.unroute(attachURL);
      const renamed = `${original} updated`;
      await page.locator(`[data-action=edit-tag][data-tag-id="${created.id}"]`).click();
      const modal = page.locator('#tag-edit-modal');
      await modal.locator('#tag-edit-name').fill(renamed);
      await modal.getByRole('radio', { name: /^Blue / }).click();
      let releaseSave!: () => void;
      const heldSave = new Promise<void>(resolve => { releaseSave = resolve; });
      const tagURL = `**/api/tags/${created.id}`;
      let writes = 0;
      await page.route(tagURL, async route => { writes++; await heldSave; await route.continue(); });
      await modal.locator('#tag-edit-save').click();
      await expect(modal).toHaveAttribute('aria-busy', 'true');
      await expect(modal.locator('#tag-edit-save')).toHaveText('Saving…');
      await expect(modal.locator('#tag-edit-cancel')).toBeDisabled();
      await page.screenshot({ path: test.info().outputPath(`tag-saving-${colorScheme}.png`) });
      await modal.locator('#tag-edit-save').evaluate(button => (button as HTMLButtonElement).click());
      releaseSave();
      await expect(modal).not.toBeVisible();
      expect(writes).toBe(1);
      await page.unroute(tagURL);
      await expect(page.locator('#tag-status')).toHaveText('Tag saved across your library.');
      for (const artifact of [first, second]) {
        const saved = await (await api.get(`/api/artifacts/${artifact.id}`)).json();
        expect(saved.tags.find((tag: { id: string }) => tag.id === created.id)).toMatchObject({ name: renamed, color: '#527cb0' });
      }
      await expect(page.locator('#save-button')).toBeDisabled();
      await page.locator(`[data-action=edit-tag][data-tag-id="${created.id}"]`).click();
      const retried = `${original} retried`;
      await modal.locator('#tag-edit-name').fill(retried);
      await page.route(tagURL, route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Temporary tag save failure. Try again.' }) }));
      await modal.locator('#tag-edit-save').click();
      await expect(modal.locator('#tag-edit-error')).toHaveText('Temporary tag save failure. Try again.');
      await expect(modal.locator('#tag-edit-name')).toHaveValue(retried);
      await expect(modal.locator('#tag-edit-save')).toBeEnabled();
      await page.screenshot({ path: test.info().outputPath(`tag-save-error-${colorScheme}.png`) });
      await page.unroute(tagURL);
      await modal.locator('#tag-edit-save').click();
      await expect(modal).not.toBeVisible();
      await expect(page.locator(`[data-tag-name="${retried}"]`)).toBeFocused();
      await page.screenshot({ path: test.info().outputPath(`tag-saved-${colorScheme}.png`) });
      await page.locator('#tag-add-name').fill(retried);
      await page.locator('#tag-create-confirm').click();
      await expect(page.locator('#tag-create-name-error')).toBeVisible();
      await expect(page.locator('#tag-add-name')).toBeFocused();
      await expect(page.locator('#tag-add-name')).toHaveAttribute('aria-invalid', 'true');
      await page.screenshot({ path: test.info().outputPath(`tag-name-error-${colorScheme}.png`) });
      await page.locator(`[data-action=detach-tag][data-tag-id="${created.id}"]`).click();
      await expect(page.locator('#tag-status')).toHaveText('Tag removed from this artifact.');
      await expect(page.locator(`[data-action=edit-tag][data-tag-id="${created.id}"]`)).toHaveCount(0);
      expect((await (await api.get(`/api/artifacts/${first.id}`)).json()).tags).toHaveLength(1);
      expect((await (await api.get(`/api/artifacts/${second.id}`)).json()).tags).toHaveLength(0);
      await page.locator('#tag-add-select').selectOption(created.id);
      await page.locator('#tag-add-confirm').click();
      await page.locator(`[data-action=edit-tag][data-tag-id="${created.id}"]`).click();
      await modal.locator('#tag-edit-delete').click();
      await page.route(tagURL, route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Temporary tag deletion failure. Try again.' }) }));
      await modal.locator('#tag-edit-confirm-delete').click();
      await expect(modal.locator('#tag-delete-error')).toHaveText('Temporary tag deletion failure. Try again.');
      await expect(modal.locator('#tag-edit-confirm-delete')).toBeEnabled();
      await page.screenshot({ path: test.info().outputPath(`tag-delete-error-${colorScheme}.png`) });
      await page.unroute(tagURL);
      await modal.locator('#tag-delete-cancel').click();
      await modal.locator('#tag-edit-delete').click();
      await expect(modal.locator('#tag-delete-error')).toBeHidden();
      await modal.locator('#tag-edit-confirm-delete').click();
      await expect(modal).not.toBeVisible();
      await expect(page.locator('#tag-status')).toHaveText('Tag deleted from your library.');
      remaining.delete(created.id);
      for (const artifact of [first, second]) expect((await (await api.get(`/api/artifacts/${artifact.id}`)).json()).tags).toHaveLength(0);
      await page.screenshot({ path: test.info().outputPath(`tag-deleted-${colorScheme}.png`) });
    }
  } finally {
    for (const id of remaining) await api.delete(`/api/tags/${id}`);
  }
});
