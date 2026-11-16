import { defineConfig } from '@playwright/test';
import base from './playwright.config';
process.env.EXHIBIT_E2E_EMBED_ORIGINS = 'http://127.0.0.1:18382';
export default defineConfig({
 ...base,
 webServer:[base.webServer as any,{command:'node embed-server.mjs',url:'http://127.0.0.1:18382/',reuseExistingServer:false}],
 testMatch: 'attribution.spec.ts',
 projects: [{name:'chromium',use:{browserName:'chromium'}},{name:'webkit',use:{browserName:'webkit'}}],
});
