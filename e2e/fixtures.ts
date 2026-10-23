import { randomUUID } from 'node:crypto';
import { test as base, expect, type APIRequestContext, type BrowserContext } from '@playwright/test';
import { appOrigin, renderOrigin, serviceToken, owner } from './environment';

export async function login(context: BrowserContext) {
  const response = await context.request.post(`${appOrigin}/auth/local`, {
    form: { ...owner, next: '/' },
  });
  expect(response.ok(), await response.text()).toBeTruthy();
  expect((await context.cookies(appOrigin)).some(cookie => cookie.name === 'exhibit_session')).toBeTruthy();
}

type Artifact = { id: string; title: string };
type Artifacts = {
  create: (body: string, title?: string) => Promise<Artifact>;
  track: (artifact: Artifact) => Artifact;
};

export const test = base.extend<{ api: APIRequestContext; artifacts: Artifacts }>({
  page: async ({ page, context }, use) => {
    // Confirm both listeners before any authenticated navigation.
    const ready = await context.request.get(`${renderOrigin}/favicon.ico`);
    expect(ready.ok()).toBeTruthy();
    await login(context);
    await use(page);
  },
  api: async ({ playwright, page }, use) => {
    // page establishes owner 1 before API seeding; this client alone has bearer auth.
    const api = await playwright.request.newContext({
      baseURL: appOrigin,
      extraHTTPHeaders: { Authorization: `Bearer ${serviceToken}` },
    });
    await use(api);
    await api.dispose();
  },
  artifacts: async ({ api }, use) => {
    const ids = new Set<string>();
    const track = (artifact: Artifact) => { ids.add(artifact.id); return artifact; };
    await use({
      track,
      async create(body, title = `E2E ${randomUUID()}`) {
        const response = await api.post('/api/artifacts', { data: { body, title } });
        expect(response.status(), await response.text()).toBe(201);
        const result = await response.json();
        return track(result.artifact ?? result);
      },
    });
    for (const id of ids) {
      const response = await api.delete(`/api/artifacts/${id}`);
      expect(response.ok(), await response.text()).toBeTruthy();
    }
  },
});
export { expect };
