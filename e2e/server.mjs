import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { access, mkdir, mkdtemp, open, rm } from 'node:fs/promises';
import { constants } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';

// Fail before touching logs or starting a process if either test port is in use.
// The Go process also fails on bind errors, covering a race after this check.
for (const port of [18080, 18081]) {
  const probe = createServer();
  await new Promise((resolve, reject) => {
    probe.once('error', reject);
    probe.listen(port, '127.0.0.1', resolve);
  }).catch(error => { throw new Error(`E2E port ${port} unavailable; stop the conflicting process.`, { cause: error }); });
  await new Promise(resolve => probe.close(resolve));
}

const binary = fileURLToPath(new URL('../bin/server', import.meta.url));
await access(binary, constants.X_OK).catch(() => { throw new Error('Missing bin/server; run make build before make e2e.'); });
const artifacts = fileURLToPath(new URL('./artifacts/', import.meta.url));
await mkdir(artifacts, { recursive: true });
const log = await open(join(artifacts, 'server.log'), 'w');
const dataDir = await mkdtemp(join(tmpdir(), 'exhibit-e2e-'));

// An allowlist of environment variables isolates the app from deployment
// credentials and settings. Empty PATH disables the optional Pi subprocess.
const child = spawn(binary, [], {
  env: {
    PATH: dataDir,
    TZ: 'UTC',
    DATA_DIR: dataDir,
    ADDR: '127.0.0.1:18080',
    RENDER_ADDR: '127.0.0.1:18081',
    APP_ORIGIN: 'http://127.0.0.1:18080',
    RENDER_ORIGIN: 'http://127.0.0.1:18081',
    AUTH_TOKEN: 'exhibit-e2e-only-token',
  },
  stdio: ['ignore', log.fd, log.fd],
});
let stopping = false;
function stop() {
  stopping = true;
  child.kill('SIGTERM');
}
process.on('SIGINT', stop);
process.on('SIGTERM', stop);
try {
  const [code, signal] = await once(child, 'exit');
  if (!stopping) {
    console.error(`E2E server exited (${code ?? signal}); see e2e/artifacts/server.log.`);
    process.exitCode = code || 1;
  }
} finally {
  await log.close();
  await rm(dataDir, { recursive: true, force: true });
}
