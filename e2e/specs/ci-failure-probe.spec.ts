import { test, expect } from '../fixtures';

test('temporary proof that hosted failures retain browser evidence', async ({ page }) => {
  await page.goto('/');
  expect('intentional CI failure probe').toBe('failure evidence captured');
});
