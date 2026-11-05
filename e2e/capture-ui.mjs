// Capture authenticated review evidence with the repository's browser runtime.
// Use only a disposable development instance; see docs/reviews/Exh-zrjo/brief.md.
import { chromium } from '@playwright/test';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';

const output = process.argv[2];
if (!output) throw new Error('Usage: node e2e/capture-ui.mjs <output-directory>');
const origin = process.env.EXHIBIT_UI_ORIGIN || 'http://t3code:18280';
const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({ reducedMotion: 'reduce' });
const captures = [];
async function prepareWordSelection(page, draft) {
  await page.locator('[data-mode=paste]').click();
  const source = page.getByRole('textbox', { name: 'HTML source', exact: true });
  await source.fill(draft);
  await source.press('ControlOrMeta+Home');
  const point = await source.evaluate(element => {
    const text = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    for (let node = text.nextNode(); node; node = text.nextNode()) {
      const start = node.textContent.indexOf('Preserve');
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
  if (await page.evaluate(() => getSelection()?.toString()) !== 'Preserve') throw new Error('Word selection failed');
}
try {
  const login = await context.request.post(`${origin}/auth/local`, {
    form: { username: process.env.EXHIBIT_UI_USER || 'admin', password: process.env.EXHIBIT_UI_PASSWORD || 'changeme', next: '/' },
  });
  if (!login.ok() || !new URL(login.url()).pathname.match(/^\/$/)) throw new Error('Fixture login failed');
  const response = await context.request.get(`${origin}/api/artifacts`);
  const artifacts = await response.json();
  const artifact = artifacts.find(item => item.title === 'Run Log');
  if (!artifact) throw new Error('Seed dev/samples before capture');
  const editorPath = `/artifacts/${artifact.id}/edit`;
  const routes = [
    ['gallery', '/'], ['search', '/search'], ['search-results', '/search?q=run'],
    ['global-search-empty', '/search?q=not-in-this-library'], ['add', '/new'], ['settings', '/settings'],
    ['editor', editorPath], ['viewer', `/artifacts/${artifact.id}`],
    ['shared', '/shared'], ['activity', '/activity'],
    ['search-empty', '/search?q=not-in-this-library'], ['filters', '/search?tag=Health'],
    ['filters-empty', '/search?tag=Health&tag=Reading'],
  ];
  await mkdir(output, { recursive: true });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  async function capture(name, route, width, scheme, prepare, height = width > 767 ? 800 : 844) {
    const file = `${name}-${width}-${scheme}.png`;
    if (captures.some(capture => capture.file === file)) return;
    await page.setViewportSize({ width, height });
    await page.emulateMedia({ colorScheme: scheme });
    await page.mouse.move(1, 1);
    await page.goto(origin + route);
    if (new URL(page.url()).pathname !== new URL(route, origin).pathname) throw new Error(`Unexpected route: ${page.url()}`);
    const marker = route === `/artifacts/${artifact.id}` ? '#pip' : 'main';
    await page.locator(marker).waitFor({ state: 'visible' });
    await page.evaluate(() => document.fonts.ready);
    if (prepare) await prepare(page);
    await page.waitForTimeout(700);
    const media = await page.evaluate(() => ({
      dark: matchMedia('(prefers-color-scheme: dark)').matches,
      scheme: getComputedStyle(document.documentElement).colorScheme,
    }));
    if (media.dark !== (scheme === 'dark')) throw new Error(`Incorrect color-scheme emulation: ${scheme}`);
    const fits = await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth);
    if (!fits) throw new Error(`Horizontal overflow: ${route} at ${width}, ${scheme}`);
    await page.screenshot({ path: path.join(output, file) });
    const assetVersion = await page.locator('link[rel=stylesheet]').first().getAttribute('href');
    captures.push({ file, route, width, height, scheme, media, assetVersion, title: await page.title() });
  }
  for (const [width, scheme] of [[1280, 'light'], [1280, 'dark'], [390, 'light'], [390, 'dark']]) {
    for (const [name, route] of routes) await capture(name, route, width, scheme);
  }
  for (const scheme of ['light', 'dark']) {
    for (const [name, route] of [['gallery', '/'], ['shared', '/shared'], ['activity', '/activity']]) {
      await capture(name, route, 768, scheme);
    }
  }
  for (const scheme of ['light', 'dark']) {
    await capture('add-paste', '/new', 390, scheme, async page => {
      await page.locator('[data-mode=paste]').click();
    });
    await capture('add-url', '/new', 390, scheme, async page => {
      await page.locator('[data-mode=url]').click();
    });
    await capture('duplicate', '/search', 390, scheme, async page => {
      await page.getByRole('button', { name: 'Actions for Run Log', exact: true }).click();
      await page.getByRole('menuitem', { name: 'Duplicate', exact: true }).click();
    });
    await capture('card-menu', '/search', 1280, scheme, async page => {
      await page.getByRole('button', { name: 'Actions for Run Log', exact: true }).click();
    });
    await capture('share', `/artifacts/${artifact.id}`, 390, scheme, async page => {
      await page.getByRole('button', { name: 'Share', exact: true }).click();
    });
    await capture('history', editorPath, 320, scheme, async page => {
      await page.locator('#versions-panel > summary').click();
      await page.locator('#versions-panel').scrollIntoViewIfNeeded();
    });
    await capture('add-error', '/new', 320, scheme, async page => {
      await page.locator('[data-mode=paste]').click();
      await page.getByRole('button', { name: 'Add to library', exact: true }).click();
    });
    await capture('viewer', `/artifacts/${artifact.id}`, 320, scheme);
    for (const width of [1280, 390, 320]) {
      await capture('source-selection', '/new', width, scheme, async page => {
        await page.locator('[data-mode=paste]').click();
        const source = page.getByRole('textbox', { name: 'HTML source', exact: true });
        await source.fill(String.raw`<!doctype html>
<!-- Readable syntax while selected -->
<section class="demo">Text</section>
<style>.demo { color: #abc; padding: 12px; content: "\n"; }</style>
<script>const count = 12; const matcher = /value+/; const escaped = "\n";</script>`);
        await source.press('ControlOrMeta+A');
        await page.locator('.cm-editor.cm-focused .cm-selectionBackground').first().waitFor({ state: 'visible' });
        await page.locator('.cm-editor.cm-focused .cm-scroller').evaluate(element => element.scrollTo(0, 0));
      });
      await page.getByRole('textbox', { name: 'HTML source', exact: true }).fill('');
      await capture('source-word-selection', '/new', width, scheme, async page => {
        await prepareWordSelection(page, '<!doctype html>\n<!-- Preserve this selection through theme changes -->\n<section>Text</section>');
      });
      await page.getByRole('textbox', { name: 'HTML source', exact: true }).fill('');
      await capture('source-occurrences', '/new', width, scheme, async page => {
        await prepareWordSelection(page, '<!doctype html>\n<!-- Preserve this selection through theme changes -->\n<section>Preserve</section>');
        await page.locator('.cm-selectionMatch').first().waitFor({ state: 'visible' });
      });
      await page.getByRole('textbox', { name: 'HTML source', exact: true }).fill('');
      await capture('source-find', '/new', width, scheme, async page => {
        await page.locator('[data-mode=paste]').click();
        const source = page.getByRole('textbox', { name: 'HTML source', exact: true });
        await source.fill('<p>Preserve this text</p>\n<p>Preserve it twice</p>');
        await source.press('ControlOrMeta+f');
        await page.locator('.cm-search [name=search]').fill('Preserve');
        await page.locator('.cm-search [name=search]').press('Tab');
        await page.locator('.cm-search [name=next]').click();
        await page.locator('.cm-search').scrollIntoViewIfNeeded();
      });
      await page.getByRole('textbox', { name: 'HTML source', exact: true }).fill('');
      await capture('source-go-line', '/new', width, scheme, async page => {
        await page.locator('[data-mode=paste]').click();
        const source = page.getByRole('textbox', { name: 'HTML source', exact: true });
        await source.fill('<p>Line one</p>\n<p>Line two</p>');
        await source.press('ControlOrMeta+Alt+g');
        await page.locator('.cm-dialog').waitFor({ state: 'visible' });
      });
      await page.getByRole('textbox', { name: 'HTML source', exact: true }).fill('');
      await capture('source-completion', '/new', width, scheme, async page => {
        await page.locator('[data-mode=paste]').click();
        const source = page.getByRole('textbox', { name: 'HTML source', exact: true });
        await source.fill('<di');
        await source.press('Control+Space');
        await page.locator('.cm-tooltip-autocomplete').waitFor({ state: 'visible' });
      });
      await page.getByRole('textbox', { name: 'HTML source', exact: true }).fill('');
    }
  }
  // The target9 pass changes shared forms/sheets and native editor chrome.
  // Capture their intentional compositions at the intermediate widths too.
  for (const scheme of ['light', 'dark']) {
    for (const width of [1280, 768, 390, 320]) {
      for (const [name, route] of [['add', '/new'], ['settings', '/settings'], ['global-search-empty', '/search?q=not-in-this-library']]) {
        await capture(name, route, width, scheme);
      }
      await capture('duplicate', '/search', width, scheme, async page => {
        await page.getByRole('button', { name: 'Actions for Run Log', exact: true }).click();
        await page.getByRole('menuitem', { name: 'Duplicate', exact: true }).click();
      });
      await capture('tag-dialog', editorPath, width, scheme, async page => {
        await page.locator('#tags-panel > summary').click();
        await page.getByRole('button', { name: 'Edit tag Health', exact: true }).click();
        await page.locator('#tag-edit-modal [aria-pressed=true]').waitFor({ state: 'visible' });
      });
      await capture('share', `/artifacts/${artifact.id}`, width, scheme, async page => {
        await page.getByRole('button', { name: 'Share', exact: true }).click();
      });
      await capture('share-bottom', `/artifacts/${artifact.id}`, width, scheme, async page => {
        await page.getByRole('button', { name: 'Share', exact: true }).click();
        await page.locator('#share-panel .modal-body').hover();
        await page.mouse.wheel(0, 1400);
        await page.waitForFunction(() => document.querySelector('#share-panel .modal-body')?.scrollTop > 0);
        const reached = await page.locator('#state-how-it-works').evaluate(element => {
          const bounds = element.getBoundingClientRect();
          const body = element.closest('.modal-body').getBoundingClientRect();
          return bounds.top >= body.top && bounds.bottom <= body.bottom;
        });
        if (!reached) throw new Error(`Share lower settings remain clipped at ${width}, ${scheme}`);
      });
      await capture('history', editorPath, width, scheme, async page => {
        await page.locator('#versions-panel > summary').click();
        await page.locator('#versions-panel').scrollIntoViewIfNeeded();
      });
      await capture('card-posture', '/search', width, scheme, async page => {
        await page.locator(`[data-href="/artifacts/${artifact.id}"] [data-capability-trigger]`).click();
        await page.locator('.capability-wrap.is-open .capability-popover').waitFor({ state: 'visible' });
      });
      await capture('viewer-posture', `/artifacts/${artifact.id}`, width, scheme, async page => {
        await page.locator('.pip-title').click();
        await page.locator('#pip-info').waitFor({ state: 'visible' });
      });
      await capture('editor-tags', editorPath, width, scheme, async page => {
        await page.locator('#tags-panel > summary').click();
        await page.locator('#tags-panel').evaluate(element => {
          element.scrollIntoView({ block: 'start' });
          window.scrollBy(0, -72);
        });
      });
      for (const section of ['security', 'state', 'assets', 'widget']) {
        await capture(`editor-${section}`, editorPath, width, scheme, async page => {
          await page.locator(`#${section}-panel > summary`).click();
          await page.locator(`#${section}-panel`).evaluate(element => {
            element.scrollIntoView({ block: 'start' });
            window.scrollBy(0, -72);
          });
        });
      }
      await capture('editor-state-actions', editorPath, width, scheme, async page => {
        await page.locator('#state-panel > summary').click();
        await page.locator('.state-row').first().waitFor({ state: 'visible' });
        await page.locator('.state-actions').evaluate(element => {
          element.scrollIntoView({ block: 'start' });
          window.scrollBy(0, -88);
        });
      });
      await capture('editor-property-adder', editorPath, width, scheme, async page => {
        await page.locator('#state-panel > summary').click();
        await page.locator('.state-adder').first().waitFor({ state: 'visible' });
        await page.locator('.state-adder').first().evaluate(element => {
          element.scrollIntoView({ block: 'start' });
          window.scrollBy(0, -88);
        });
      });
      if (width < 768) {
        await capture('filter-sheet', '/search', width, scheme, async page => {
          await page.locator('.gallery-filter-trigger').click();
        });
        await capture('add-paste-footer', '/new', width, scheme, async page => {
          await page.locator('[data-mode=paste]').click();
          await page.locator('#ingest-panel .ingest-foot').scrollIntoViewIfNeeded();
        });
      }
    }
    for (const [width, height] of [[320, 640], [390, 640], [640, 360], [1280, 400]]) {
      await capture('share-short', `/artifacts/${artifact.id}`, width, scheme, async page => {
        await page.getByRole('button', { name: 'Share', exact: true }).click();
        if (await page.locator('#share-panel .modal-body').evaluate(element => element.scrollTop) !== 0)
          throw new Error(`Share did not open at the top at ${width}x${height}, ${scheme}`);
      }, height);
    }
  }
  await writeFile(path.join(output, 'manifest.json'), JSON.stringify({ capturedAt: new Date().toISOString(), captures, errors }, null, 2) + '\n');
  if (errors.length) throw new Error(`Browser errors: ${errors.join('; ')}`);
  console.log(`Captured ${captures.length} verified app states in ${output}`);
} finally {
  await browser.close();
}
