// Keep Clerk's short-lived cookie fresh on every authenticated app page.
import './controllers.js';
import { clerkClient } from './clerk-client.js';

clerkClient().catch(() => {
  // Auth controls show an actionable error; protected requests still fail closed.
});
