import { test, expect } from '../fixtures';

test('source editor remains legible through actual light/dark changes', async ({ page }) => {
  await page.goto('/new');
  await page.locator('[data-mode=paste]').click();
  const source = page.getByRole('textbox', { name: 'HTML source', exact: true });
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme });
    await expect.poll(() => page.evaluate(() => matchMedia('(prefers-color-scheme: dark)').matches)).toBe(colorScheme === 'dark');
    await source.fill('');
    await expect(page.locator('.cm-placeholder')).toBeVisible();
    const placeholderContrast = await page.locator('.cm-placeholder').evaluate(element => {
      const luminance = (color: string) => {
        const channels = color.match(/[\d.]+/g)!.slice(0, 3).map(Number).map(value => {
          value /= 255;
          return value <= .04045 ? value / 12.92 : ((value + .055) / 1.055) ** 2.4;
        });
        return channels.reduce((sum, value, i) => sum + value * [.2126, .7152, .0722][i], 0);
      };
      const foreground = luminance(getComputedStyle(element).color);
      const editor = element.closest('.cm-editor')!;
      const field = getComputedStyle(editor).backgroundColor;
      const line = getComputedStyle(editor.querySelector('.cm-activeLine')!).backgroundColor.match(/[\d.]+/g)!.map(Number);
      const base = field.match(/[\d.]+/g)!.map(Number), alpha = line[3] ?? 1;
      const active = `rgb(${base.slice(0, 3).map((channel, i) => line[i] * alpha + channel * (1 - alpha)).join(',')})`;
      const backgrounds = [field, active];
      return Math.min(...backgrounds.map(color => {
        const background = luminance(color);
        return (Math.max(foreground, background) + .05) / (Math.min(foreground, background) + .05);
      }));
    });
    expect(placeholderContrast).toBeGreaterThanOrEqual(4.5);
    await source.fill('<!doctype html>\n<!-- Keep this draft across theme changes -->');
    await page.emulateMedia({ colorScheme: colorScheme === 'dark' ? 'light' : 'dark' });
    await expect(source).toContainText('Keep this draft across theme changes');
    const fonts = await page.locator('#ingest-panel').evaluate(panel => {
      const bodyFont = getComputedStyle(document.body).fontFamily;
      return [...panel.querySelectorAll('button,input:not([type=checkbox]),a.btn')].every(element => getComputedStyle(element).fontFamily === bodyFont);
    });
    expect(fonts).toBe(true);
  }
});

test('source syntax stays differentiated and readable in both color schemes', async ({ page, artifacts }) => {
  const draft = String.raw`<!doctype html>
<!-- Preserve syntax and this draft across theme changes -->
<section class="demo">Text</section>
<style>.demo { color: #abc; padding: 12px; content: "\n"; }</style>
<script>const count = 12; const matcher = /value+/; const escaped = "\n";</script>`;
  const artifact = await artifacts.create(draft);
  for (const [route, label] of [['/new', 'HTML source'], [`/artifacts/${artifact.id}/edit`, 'Artifact source']] as const) {
    await page.goto(route);
    if (route === '/new') await page.locator('[data-mode=paste]').click();
    const source = page.getByRole('textbox', { name: label, exact: true });
    if (route === '/new') await source.fill(draft);
    for (const width of [1280, 390, 320]) {
      await page.setViewportSize({ width, height: 844 });
      for (const colorScheme of ['light', 'dark', 'light'] as const) {
        await page.emulateMedia({ colorScheme });
        await expect.poll(() => page.evaluate(() => matchMedia('(prefers-color-scheme: dark)').matches)).toBe(colorScheme === 'dark');
        await source.press('ControlOrMeta+A');
        await expect(page.locator('.cm-editor.cm-focused .cm-selectionBackground').first()).toBeVisible();
        await expect.poll(() => page.locator('.cm-editor.cm-focused .cm-content span').evaluateAll(elements => new Set(elements.map(element => getComputedStyle(element).color)).size)).toBeGreaterThanOrEqual(4);
        const appearance = await page.locator('.cm-editor.cm-focused').evaluate(editor => {
          const luminance = (color: string) => color.match(/[\d.]+/g)!.slice(0, 3).map(Number).reduce((sum, value, index) => {
            const channel = value / 255;
            return sum + (channel <= .04045 ? channel / 12.92 : ((channel + .055) / 1.055) ** 2.4) * [.2126, .7152, .0722][index];
          }, 0);
          const composite = (foreground: string, background: string) => {
            const front = foreground.match(/[\d.]+/g)!.map(Number), back = background.match(/[\d.]+/g)!.map(Number);
            const alpha = front[3] ?? 1;
            return `rgb(${back.slice(0, 3).map((channel, i) => front[i] * alpha + channel * (1 - alpha)).join(',')})`;
          };
          const selectionBackground = getComputedStyle(editor.querySelector('.cm-selectionBackground')!).backgroundColor;
          const field = getComputedStyle(editor).backgroundColor, active = getComputedStyle(editor.querySelector('.cm-activeLine')!).backgroundColor;
          const surfaces = [field, composite(active, field), selectionBackground, composite(active, selectionBackground)];
          const tokens = [...editor.querySelectorAll('.cm-content span')].filter(element => element.textContent?.trim());
          const tokenColor = (text: string) => {
            const token = tokens.find(element => element.textContent === text);
            return token ? getComputedStyle(token).color : null;
          };
          return {
            selectionBackground,
            tag: tokenColor('section'),
            string: tokenColor('"demo"'),
            lowContrast: tokens.flatMap(element => surfaces.flatMap(surface => {
              const foreground = luminance(getComputedStyle(element).color);
              const background = luminance(surface);
              const ratio = (Math.max(foreground, background) + .05) / (Math.min(foreground, background) + .05);
              return ratio < 4.5 ? [{ text: element.textContent, ratio }] : [];
            })),
          };
        });
        expect(appearance.tag).not.toBeNull();
        expect(appearance.string).not.toBeNull();
        expect(appearance.tag).not.toBe(appearance.string);
        expect(appearance.selectionBackground).not.toBe('rgba(0, 0, 0, 0)');
        expect(appearance.lowContrast, `${width}px ${colorScheme} selection ${appearance.selectionBackground}`).toEqual([]);
        await expect(source).toContainText('Preserve syntax and this draft across theme changes');
      }
    }
    if (route === '/new') await source.fill('');
  }
});

test('word selection remains visible on the active source line', async ({ page, artifacts }) => {
  const draft = '<!doctype html>\n<!-- Preserve this selection through theme changes -->\n<section>Preserve</section>';
  const artifact = await artifacts.create(draft);
  for (const [route, label] of [['/new', 'HTML source'], [`/artifacts/${artifact.id}/edit`, 'Artifact source']] as const) {
    await page.goto(route);
    if (route === '/new') await page.locator('[data-mode=paste]').click();
    const source = page.getByRole('textbox', { name: label, exact: true });
    if (route === '/new') await source.fill(draft);
    for (const width of [1280, 390, 320]) {
      await page.setViewportSize({ width, height: 844 });
      for (const colorScheme of ['light', 'dark', 'light'] as const) {
        await page.emulateMedia({ colorScheme });
        await expect.poll(() => page.evaluate(() => matchMedia('(prefers-color-scheme: dark)').matches)).toBe(colorScheme === 'dark');
        await source.press('ControlOrMeta+Home');
        await source.scrollIntoViewIfNeeded();
        const point = await source.evaluate(element => {
          const text = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
          for (let node = text.nextNode(); node; node = text.nextNode()) {
            const start = node.textContent!.indexOf('Preserve');
            if (start < 0) continue;
            const range = document.createRange();
            range.setStart(node, start + 1);
            range.setEnd(node, start + 2);
            const rect = range.getBoundingClientRect();
            return { x: rect.x + rect.width / 2, y: rect.y + rect.height / 2 };
          }
          throw new Error('Selection word is missing');
        });
        await page.mouse.dblclick(point.x, point.y);
        await expect.poll(() => page.evaluate(() => getSelection()?.toString())).toBe('Preserve');
        await expect(page.locator('.cm-editor.cm-focused .cm-activeLine')).toContainText('Preserve');
        await expect(page.locator('.cm-editor.cm-focused .cm-selectionMatch').first()).toBeVisible();
        const paint = await page.locator('.cm-editor.cm-focused').evaluate(editor => {
          const range = getSelection()!.getRangeAt(0);
          const rect = range.getBoundingClientRect();
          const x = rect.x + rect.width / 2, y = rect.y + rect.height / 2;
          const selection = [...editor.querySelectorAll<HTMLElement>('.cm-selectionBackground')].find(element => {
            const box = element.getBoundingClientRect();
            return x >= box.left && x < box.right && y >= box.top && y < box.bottom;
          })!;
          // Include the non-interactive selection rectangle in hit testing without
          // changing its painting, so the real front-to-back stack can be composed.
          const pointerEvents = selection.style.pointerEvents;
          selection.style.pointerEvents = 'auto';
          const stack = document.elementsFromPoint(x, y);
          selection.style.pointerEvents = pointerEvents;
          const layers = stack.slice(0, stack.indexOf(editor) + 1);
          const color = (value: string) => value.match(/[\d.]+/g)!.map(Number);
          const composite = (includeSelection: boolean, paintLayers = layers) => paintLayers.slice().reverse().reduce((background, element) => {
            if (!includeSelection && element === selection) return background;
            const style = getComputedStyle(element), foreground = color(style.backgroundColor);
            const alpha = (foreground[3] ?? 1) * Number(style.opacity);
            return background.map((channel, i) => foreground[i] * alpha + channel * (1 - alpha));
          }, [255, 255, 255]);
          const luminance = (channels: number[]) => channels.reduce((sum, value, i) => {
            const channel = value / 255;
            return sum + (channel <= .04045 ? channel / 12.92 : ((channel + .055) / 1.055) ** 2.4) * [.2126, .7152, .0722][i];
          }, 0);
          const selected = composite(true), unselected = composite(false);
          const matchRect = editor.querySelector('.cm-selectionMatch')!.getBoundingClientRect();
          const matchStack = document.elementsFromPoint(matchRect.x + matchRect.width / 2, matchRect.y + matchRect.height / 2);
          const matched = composite(true, matchStack.slice(0, matchStack.indexOf(editor) + 1));
          const field = color(getComputedStyle(editor).backgroundColor).slice(0, 3);
          const foreground = luminance(color(getComputedStyle(range.startContainer.parentElement!).color).slice(0, 3));
          const background = luminance(selected);
          return {
            layerIncluded: layers.includes(selection),
            selected, unselected,
            difference: Math.max(...selected.map((value, i) => Math.abs(value - unselected[i]))),
            selectionEmphasis: Math.hypot(...selected.map((value, i) => value - field[i])),
            matchEmphasis: Math.hypot(...matched.map((value, i) => value - field[i])),
            contrast: (Math.max(foreground, background) + .05) / (Math.min(foreground, background) + .05),
          };
        });
        expect(paint.layerIncluded).toBe(true);
        expect(paint.difference, `${route} ${width}px ${colorScheme}: ${JSON.stringify(paint)}`).toBeGreaterThan(15);
        expect(paint.contrast).toBeGreaterThanOrEqual(4.5);
        expect(paint.selectionEmphasis, `${colorScheme}: selected occurrence should lead`).toBeGreaterThan(paint.matchEmphasis);
        await expect(source).toContainText('Preserve this selection through theme changes');
      }
    }
    if (route === '/new') await source.fill('');
  }
});

test('source find and line-navigation controls follow the shared UI in both themes', async ({ page, artifacts }) => {
  const draft = '<p>Preserve this text</p>\n<p>Preserve it twice</p>';
  const artifact = await artifacts.create(draft);
  for (const [route, label] of [['/new', 'HTML source'], [`/artifacts/${artifact.id}/edit`, 'Artifact source']] as const) {
    await page.goto(route);
    if (route === '/new') await page.locator('[data-mode=paste]').click();
    const source = page.getByRole('textbox', { name: label, exact: true });
    if (route === '/new') await source.fill(draft);
    const editor = page.locator('.cm-editor').filter({ has: source });
    for (const width of [1280, 390, 320]) {
      await page.setViewportSize({ width, height: 844 });
      for (const colorScheme of ['light', 'dark'] as const) {
        await page.emulateMedia({ colorScheme });
        await source.press('ControlOrMeta+f');
        const panel = editor.locator('.cm-search');
        await expect(panel).toBeVisible();
        const layout = await panel.evaluate(element => {
          const controls = [...element.querySelectorAll('input:not([type=checkbox]),button')];
          const cells = [...element.querySelectorAll(':scope > input, :scope > button, :scope > label')].map(control => control.getBoundingClientRect());
          const font = getComputedStyle(document.body).fontFamily;
          return {
            heights: controls.map(control => control.getBoundingClientRect().height),
            readable: controls.every(control => parseFloat(getComputedStyle(control).fontSize) >= 14 && getComputedStyle(control).fontFamily === font),
            flat: controls.every(control => getComputedStyle(control).backgroundImage === 'none'),
            fits: element.scrollWidth <= element.clientWidth,
            overlap: cells.some((a, i) => cells.slice(i + 1).some(b => Math.min(a.right, b.right) - Math.max(a.left, b.left) > 1 && Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top) > 1)),
            navigationSingleLine: [...element.querySelectorAll('button[name=next],button[name=prev],button[name=select]')].every(button => {
              const text = document.createRange();
              text.selectNodeContents(button);
              return new Set([...text.getClientRects()].map(rect => Math.round(rect.top))).size === 1;
            }),
            sourceHeight: element.closest('.cm-editor')!.querySelector('.cm-scroller')!.getBoundingClientRect().height,
          };
        });
        expect(Math.min(...layout.heights), `${route} ${width}px ${colorScheme}`).toBeGreaterThanOrEqual(width < 768 ? 44 : 32);
        expect(layout).toMatchObject({ readable: true, flat: true, fits: true, overlap: false, navigationSingleLine: true });
        expect(layout.sourceHeight).toBeGreaterThanOrEqual(120);
        await panel.locator('[name=search]').fill('Preserve');
        await panel.locator('[name=search]').press('Tab');
        await expect.poll(() => editor.locator('.cm-searchMatch').count()).toBeGreaterThanOrEqual(2);
        await page.emulateMedia({ colorScheme: colorScheme === 'light' ? 'dark' : 'light' });
        await expect(panel.locator('[name=search]')).toHaveValue('Preserve');
        await expect(source).toContainText('Preserve it twice');
        await page.emulateMedia({ colorScheme });
        await panel.locator('[name=next]').click();
        await expect(panel.locator('[name=next]')).toHaveText('Next');
        await panel.getByRole('button', { name: 'Close', exact: true }).click();
        await expect(panel).toHaveCount(0);
        await source.press('ControlOrMeta+Alt+g');
        const dialog = editor.locator('.cm-dialog');
        await expect(dialog).toBeVisible();
        await expect(dialog.getByRole('button', { name: 'Go', exact: true })).toBeVisible();
        await dialog.locator('input[name=line]').fill('2');
        await dialog.locator('input[name=line]').press('Enter');
        await expect(dialog).toHaveCount(0);
        await expect(source).toContainText('Preserve it twice');
      }
    }
    if (route === '/new') await source.fill('');
  }
});

test('phone share controls and choices compose cleanly in both themes', async ({ page, api, artifacts }) => {
  const artifact = await artifacts.create('<h1>Share layout fixture</h1>');
  const link = await api.post('/api/shares', { data: { artifact_id: artifact.id } });
  expect(link.ok(), await link.text()).toBeTruthy();
  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.emulateMedia({ colorScheme });
      await page.goto(`/artifacts/${artifact.id}`);
      await page.getByRole('button', { name: 'Share', exact: true }).click();
      await expect(page.locator('#share-panel')).toBeVisible();
      await page.evaluate(() => document.fonts.ready);
      const layout = await page.locator('#share-panel').evaluate(panel => {
        const rect = (selector: string) => panel.querySelector(selector)!.getBoundingClientRect();
        const replace = rect('#share-link-replace');
        const helper = rect('.share-replace-row > span');
        const link = rect('#share-link-url');
        const copy = rect('#share-link-copy');
        const choices = [...panel.querySelectorAll('.state-choice > span')].map(element => element.getBoundingClientRect());
        return {
          helperBelow: helper.top >= replace.bottom - .5,
          equalControlHeights: Math.abs(link.height - copy.height) < 1 && copy.height >= 44,
          stackedChoices: choices[1].top >= choices[0].bottom && Math.abs(choices[0].left - choices[1].left) < 1,
          wideChoices: choices.every(choice => choice.width >= 200),
          fits: panel.scrollWidth <= panel.clientWidth,
        };
      });
      expect(layout, `${width}px ${colorScheme}`).toEqual({ helperBelow: true, equalControlHeights: true, stackedChoices: true, wideChoices: true, fits: true });
      await page.locator('#share-close').click();
    }
  }
});

test('scrolling Share body clips choices behind the pinned actions', async ({ page, api, artifacts }) => {
  const artifact = await artifacts.create('<h1>Share clipping fixture</h1>');
  const link = await api.post('/api/shares', { data: { artifact_id: artifact.id } });
  expect(link.ok(), await link.text()).toBeTruthy();
  for (const width of [320, 390]) {
    await page.setViewportSize({ width, height: 844 });
    for (const colorScheme of ['light', 'dark'] as const) {
      await page.emulateMedia({ colorScheme });
      await page.goto(`/artifacts/${artifact.id}`);
      await page.getByRole('button', { name: 'Share', exact: true }).click();
      const panel = page.locator('#share-panel');
      await expect(panel).toBeVisible();
      const paint = await panel.evaluate(async dialog => {
        const body = dialog.querySelector<HTMLElement>('.modal-body')!;
        const footer = dialog.querySelector<HTMLElement>('.modal-footer')!;
        const radio = dialog.querySelector<HTMLInputElement>('#share-state-shared')!;
        const footerBefore = footer.getBoundingClientRect();
        const radioBefore = radio.getBoundingClientRect();
        const targetY = footerBefore.top + footerBefore.height / 2;
        body.scrollTop += radioBefore.top + radioBefore.height / 2 - targetY;
        await new Promise(requestAnimationFrame);
        const bounds = radio.getBoundingClientRect();
        const x = bounds.left + bounds.width / 2, y = bounds.top + bounds.height / 2;
        const footerAfter = footer.getBoundingClientRect();
        return {
          scrollable: body.scrollHeight > body.clientHeight,
          scrolled: body.scrollTop > 0,
          targetBehindFooter: y > body.getBoundingClientRect().bottom && y < footerAfter.bottom,
          footerOwnsPoint: footer.contains(document.elementFromPoint(x, y)),
          footerStable: Math.abs(footerBefore.top - footerAfter.top) < 1,
        };
      });
      expect(paint, `${width}px ${colorScheme}`).toEqual({ scrollable: true, scrolled: true, targetBehindFooter: true, footerOwnsPoint: true, footerStable: true });
      const disclosure = panel.locator('#state-how-it-works > summary');
      await disclosure.scrollIntoViewIfNeeded();
      await disclosure.click();
      await panel.locator('#state-how-it-works > p').last().scrollIntoViewIfNeeded();
      const lowerContentFits = await panel.locator('#state-how-it-works > p').last().evaluate(paragraph => {
        const bounds = paragraph.getBoundingClientRect();
        const body = paragraph.closest('.modal-body')!.getBoundingClientRect();
        return bounds.top >= body.top && bounds.bottom <= body.bottom;
      });
      expect(lowerContentFits).toBe(true);
      await expect(page.locator('#share-close')).toBeVisible();
      await page.locator('#share-close').click();
    }
  }
});
