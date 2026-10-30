// Keep Clerk's short-lived cookie fresh on every authenticated app page.
import './controllers.js';
import { clerkClientAfterLoad } from './clerk-client.js';

clerkClientAfterLoad().catch(() => {
  // Auth controls show an actionable error; protected requests still fail closed.
});
