import { test, expect } from '../fixtures';

test('expanded editor tools keep descriptions with actions and consistent operable controls', async ({ page, api, artifacts }) => {
  const artifact = await artifacts.create('<h1>Editor tools fixture</h1>');
  const tagResponse = await api.post('/api/tags', { data: { name: `Tools-${artifact.id.slice(0, 8)}`, color: '#64748b' } });
  expect(tagResponse.status(), await tagResponse.text()).toBe(201);
  const tag = await tagResponse.json();
  try {
    const attached = await api.post(`/api/tags/${tag.id}/artifacts/${artifact.id}`);
    expect(attached.ok(), await attached.text()).toBeTruthy();
    const sample = { runs: [{ distance: 5.2, label: 'Morning' }, { distance: 8, label: 'Evening' }], category: 'Road', enabled: true };
    const response = await api.put(`/api/artifacts/${artifact.id}/state`, { data: { key: 'sample', value: JSON.stringify(sample) } });
    expect(response.status(), await response.text()).toBe(204);
    await page.goto(`/artifacts/${artifact.id}/edit`);
    for (const section of ['tags', 'security', 'state', 'assets', 'widget']) await page.locator(`#${section}-panel > summary`).click();
    await expect(page.locator('.state-record')).toHaveCount(2);
    for (const width of [1280, 768, 390, 320]) {
      await page.setViewportSize({ width, height: 844 });
      for (const colorScheme of ['light', 'dark'] as const) {
        await page.emulateMedia({ colorScheme });
        await page.mouse.move(1, 1);
        await page.evaluate(() => document.fonts.ready);
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), `${width}px ${colorScheme}`).toBe(true);
        const permissionRoles = await page.locator('#security-panel').evaluate(panel => {
          const style = (element: Element) => {
            const s = getComputedStyle(element);
            return [s.fontSize, s.fontWeight, s.lineHeight, s.color];
          };
          const buttonPaint = (element: Element) => {
            const s = getComputedStyle(element);
            return [s.backgroundColor, s.color, s.borderColor];
          };
          return {
            heading: style(panel.querySelector('h3')!),
            sharedHeading: style(document.querySelector('.tag-create h3')!),
            labels: [...panel.querySelectorAll('label.row > span')].map(style),
            sharedLabel: style(document.querySelector('label[for=title]')!),
            add: buttonPaint(panel.querySelector('#al-add-btn')!),
            sharedAdd: buttonPaint(document.querySelector('.tag-add-row button')!),
          };
        });
        expect(permissionRoles.heading, `${width}px ${colorScheme} permission heading`).toEqual(permissionRoles.sharedHeading);
        expect(permissionRoles.labels).toHaveLength(6);
        for (const label of permissionRoles.labels) expect(label).toEqual(permissionRoles.sharedLabel);
        expect(permissionRoles.add).toEqual(permissionRoles.sharedAdd);
        await expect(page.locator('#allowlist-rows')).toContainText('No origins approved.');
        const toggleTargets = await page.locator('.state-toggle').evaluateAll(elements => elements.map(element => element.getBoundingClientRect().height));
        expect(toggleTargets.length).toBeGreaterThan(0);
        expect(Math.min(...toggleTargets), `${width}px ${colorScheme} boolean target`).toBeGreaterThanOrEqual(44);
        const tagButtons = await page.locator('.tag-pill-btn').evaluateAll(elements => elements.map(element => {
          const bounds = element.getBoundingClientRect();
          return { width: bounds.width, height: bounds.height };
        }));
        expect(tagButtons).toHaveLength(2);
        expect(tagButtons.every(bounds => bounds.width === bounds.height && bounds.height >= (width < 768 ? 44 : 32)), `${width}px ${colorScheme} tag actions`).toBe(true);
        const tagGeometry = await page.locator('.tag-edit-item').first().evaluate(item => {
          const chip = item.querySelector('.tag-pill')!.getBoundingClientRect();
          const actions = item.querySelector('.tag-edit-actions')!.getBoundingClientRect();
          const swatch = document.querySelector('.color-swatch[aria-pressed=true]')!;
          const outer = getComputedStyle(swatch), badge = getComputedStyle(swatch, '::after');
          return { separated: chip.right <= actions.left, compact: chip.height < actions.height,
            inset: parseFloat(badge.right), cornerDrift: Math.abs(parseFloat(outer.borderTopRightRadius) - parseFloat(outer.borderWidth) - parseFloat(badge.right) - parseFloat(badge.borderTopRightRadius)) };
        });
        expect(tagGeometry.separated).toBe(true);
        expect(tagGeometry.compact).toBe(true);
        expect(tagGeometry.inset).toBeGreaterThanOrEqual(2);
        expect(tagGeometry.cornerDrift).toBeLessThan(0.5);
        const propertyRowsAlign = await page.locator('.state-adder').evaluateAll(rows => rows.every(row => {
          const heights = [...row.querySelectorAll('input,select,button')].map(element => element.getBoundingClientRect().height);
          return Math.max(...heights) - Math.min(...heights) < 1;
        }));
        expect(propertyRowsAlign).toBe(true);
        if (width < 768) {
          const tagPrimaryFillsColumn = await page.locator('#tag-create-confirm').evaluate(button => {
            const action = button.getBoundingClientRect(), field = document.getElementById('tag-add-name')!.getBoundingClientRect();
            return Math.abs(action.left - field.left) < 1 && Math.abs(action.right - field.right) < 1;
          });
          expect(tagPrimaryFillsColumn).toBe(true);
        }
        const controls = await page.locator('#state-panel button,#assets-panel button,#widget-panel button').evaluateAll(elements => elements.map(element => element.getBoundingClientRect().height));
        expect(Math.min(...controls), `${width}px ${colorScheme} panel controls`).toBeGreaterThanOrEqual(width < 768 ? 44 : 36);
        const icons = await page.locator('.state-record-head > button').evaluateAll(elements => elements.map(element => {
          const bounds = element.getBoundingClientRect();
          return { width: bounds.width, height: bounds.height };
        }));
        expect(icons.every(bounds => bounds.width === bounds.height)).toBe(true);
        expect(Math.max(...icons.map(bounds => bounds.width)) - Math.min(...icons.map(bounds => bounds.width))).toBeLessThan(1);
        const propertyActionsAlign = await page.getByRole('button', { name: 'Remove property', exact: true }).evaluateAll(elements => elements.every(element => Math.abs(element.getBoundingClientRect().right - element.closest('.state-field')!.getBoundingClientRect().right) < 1));
        expect(propertyActionsAlign).toBe(true);
        for (const prefix of ['state', 'assets']) {
          for (const operation of ['export', 'import']) {
            const layout = await page.locator(`#${prefix}-${operation}`).evaluate(button => {
              const note = document.getElementById(button.getAttribute('aria-describedby')!)!;
              const action = button.getBoundingClientRect(), help = note.getBoundingClientRect();
              return { gap: innerWidth <= 640 ? help.top - action.bottom : help.left - action.right };
            });
            expect(layout.gap).toBeGreaterThanOrEqual(7.5);
          }
        }
      }
    }
    // Server-rendered working-copy rows replace the empty note and restore it
    // when removed, without changing the artifact's saved permissions.
    await page.getByLabel('Add origin', { exact: true }).fill('https://draft.example.test');
    await page.locator('#al-add-btn').click();
    await expect(page.locator('#allowlist-rows .allowlist-row')).toHaveCount(1);
    await expect(page.locator('#allowlist-rows')).not.toContainText('No origins approved.');
    const draftRow = await page.locator('#allowlist-rows .allowlist-row').evaluate(row => {
      const origin = row.querySelector('code')!.getBoundingClientRect();
      const action = row.querySelector('button')!.getBoundingClientRect();
      return { separated: origin.bottom <= action.top, fits: row.scrollWidth <= row.clientWidth };
    });
    expect(draftRow).toEqual({ separated: true, fits: true });
    await page.locator('#allowlist-rows').getByRole('button', { name: 'Remove', exact: true }).click();
    await expect(page.locator('#allowlist-rows .allowlist-row')).toHaveCount(0);
    await expect(page.locator('#allowlist-rows')).toContainText('No origins approved.');
    const savedArtifact = await api.get(`/api/artifacts/${artifact.id}`);
    expect((await savedArtifact.json()).network_allowlist).toEqual([]);
    // The enlarged chip action still opens/cancels the native editor.
    const edit = page.getByRole('button', { name: `Edit tag ${tag.name}`, exact: true });
    await edit.click();
    await expect(page.locator('#tag-edit-modal')).toBeVisible();
    await page.locator('#tag-edit-cancel').click();
    await expect(edit).toBeFocused();
    // The larger controls still operate the working copy and Cancel discards it.
    await page.getByRole('button', { name: 'Move down', exact: true }).first().click();
    await expect(page.locator('.state-record input[type=number]').first()).toHaveValue('8');
    await page.locator('#state-cancel').click();
    await expect(page.locator('.state-record input[type=number]').first()).toHaveValue('5.2');
    const saved = await api.get(`/api/artifacts/${artifact.id}/state`);
    expect(await saved.json()).toEqual({ sample: JSON.stringify(sample) });
  } finally {
    const removed = await api.delete(`/api/tags/${tag.id}`);
    expect(removed.ok(), await removed.text()).toBeTruthy();
  }
});
