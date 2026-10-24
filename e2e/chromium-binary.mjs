import { chromium } from '@playwright/test';

// Discover and smoke-test the same headless executable Playwright will use.
// executablePath() names full Chrome, while headless launch uses the shell;
// launchServer's public process API avoids assuming OS/architecture paths.
const server = await chromium.launchServer();
try {
  console.log(server.process().spawnfile);
} finally {
  await server.close();
}
