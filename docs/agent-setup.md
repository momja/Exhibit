# Set up Exhibit with a coding agent

Give a coding agent access to a terminal **on the machine that will run Exhibit**,
then paste one of the prompts below. The agent installs the dependencies, writes
configuration, starts the service, and checks the result with you. An agent in a
remote workspace installs there, unless it already has access to your machine.

Use [Docker Compose](agent-setup-compose.md) by default. Use the
[source guide](agent-setup-source.md) when you want to build and run without
Docker. These guides target Ubuntu 24.04 LTS and macOS on ARM64 or x86-64, using
a POSIX shell. Windows users should run the Ubuntu path inside WSL2; Docker
Desktop must have integration enabled for that distribution. Native Windows and
other distributions need an adapted prerequisite step, and must be reported as
unverified until exercised. See the [validation record](agent-setup-validation.md)
for the paths actually checked.

## Copy a prompt

Docker Compose:

```text
Install and deploy Exhibit on this machine using Docker Compose. The repository
is https://github.com/momja/Exhibit. Obtain the repo safely if it is not already
here, read AGENTS.md and docs/agent-setup.md, and follow
docs/agent-setup-compose.md. Execute the setup, installing missing prerequisites
and persisting environment settings. Default to loopback-only access. Preserve
existing configuration, accounts, and data. Ask only for missing choices or
credentials, and have me enter secrets locally. Verify login, the gallery,
artifact rendering, persistence after restart, and the agent runtime. Test an
agent interaction if I provide a provider key; otherwise report that check as
pending. Finish with actual URLs, config/data paths, and stop/restart/log commands.
```

From source:

```text
Install and deploy Exhibit from source on this machine. The repository is
https://github.com/momja/Exhibit. Obtain the repo safely if it is not already here,
read AGENTS.md and docs/agent-setup.md, and follow docs/agent-setup-source.md.
Execute the setup: download/install missing dependencies, build assets and the
server, install the repository's pinned Pi runtime, save environment settings,
and start Exhibit. Default to loopback-only access. Preserve existing
configuration, accounts, and data. Ask only for missing choices or credentials,
and have me enter secrets locally. Verify login, the gallery, artifact rendering,
persistence after restart, and the agent runtime. Test an agent interaction if I
provide a provider key; otherwise report that check as pending. Finish with actual
URLs, config/data paths, and stop/restart/log commands.
```

## Instructions for the agent

Perform the work and explain progress in plain language. Use the checkout's
[deployment reference](deployment.md) for settings, [Dockerfile](../Dockerfile)
for the Pi pin and container toolchain, and [go.mod](../go.mod) for Go's minimum
version. Follow the repository's agent instructions; a local installation does
not require edits to tracked application files. Commands below assume the
checkout root, and fresh-install examples must be adapted rather than pasted
over an existing installation.

1. Inspect OS/architecture, shell, installation directory, disk space, tool
   versions, and listeners on the proposed ports. On Linux use `ss -ltn`; on
   macOS use `lsof -nP -iTCP -sTCP:LISTEN`. Check for an existing server, Compose
   project, data directory, and environment/launcher files. Inspect configuration
   names without printing secret values. Reuse an installation without resetting
   its checkout, changing its project name, or replacing its data. Do not update
   an existing instance to a new revision as a side effect of setup.
2. Default to local-only access and unused ports 8080/8081. Ask for an install
   location only if one is ambiguous, and for a network hostname only when remote
   access is wanted. Install missing tools using the platform-specific path in
   the chosen guide. If an administrator prompt, Docker Desktop GUI step, or
   reboot cannot be completed through your tools, explain that exact step and
   continue independent preparation. Do not report deployment complete until the
   prerequisite works.
3. If needed, clone `https://github.com/momja/Exhibit.git` into a new directory:

   ```sh
   git clone https://github.com/momja/Exhibit.git
   cd Exhibit
   ```

   If `Exhibit` already exists, inspect it before reusing it. Do not remove it to
   make the clone command succeed. On re-runs, reuse the original `.env.setup`,
   launcher/overlay, and data volume/path; create only missing files. Review any
   necessary setting changes individually. Keep the generated `AUTH_TOKEN` and
   `DATA_DIR/secret.key` (or an explicitly configured `EXHIBIT_SECRET`) stable:
   the latter decrypts stored provider keys and signs render credentials.
4. Follow the chosen guide, then complete the checks below. Save a brief local
   handoff with paths, commands and results, without passwords, API tokens, keys,
   or expanded environment dumps. `.env.setup` and `.exhibit-setup/` are ignored
   by Git and excluded from Docker builds. Verify that remains true in the
   checkout being installed, especially if adapting this guide to an older one.

## Verify the installation

Use the actual configured origins. For local defaults:

```sh
curl --fail --silent --output /dev/null http://localhost:8080/auth/login
curl --silent --output /dev/null --write-out '%{http_code}\n' http://localhost:8081/
```

The login page should answer 200. A 404 at the renderer's root is expected;
confirm the real render document in step 2. A successful TCP connection alone
does not establish that artifacts or the agent work.

1. Sign in. A fresh default instance creates `admin` / `changeme`; use the
   chosen guide's `user passwd admin` command immediately, entering the new
   password in the local terminal with echo disabled or through the Accounts UI.
   Existing instances keep their credentials. Check that `/api/artifacts` answers
   401 without a credential and 200 with the logged-in browser session.
2. Import this file through **New** (paste HTML or upload a file), open it, and
   press the button. Confirm that the iframe's document comes from
   `RENDER_ORIGIN`, different from the gallery's origin, and shows no failed
   render requests. It needs no network approvals:

   ```html
   <!doctype html><html><head><meta charset="utf-8"><title>Setup check</title></head>
   <body><button id="save">Save setup check</button><output id="result"></output>
   <script>
   const result = document.getElementById('result');
   result.textContent = localStorage.getItem('setup-check') || 'Not saved';
   document.getElementById('save').onclick = () => {
     localStorage.setItem('setup-check', 'Saved');
     result.textContent = 'Saved';
   };
   </script></body></html>
   ```

3. Wait for the state write to finish, reload, and confirm **Saved**. Restart the
   service using the chosen guide, reload again, and confirm both the artifact
   and its saved value remain. Re-run the launch command once more and confirm
   the same library, token, config files and data path/volume are still in use.
4. Check `pi --version` in the server's runtime (the container for Compose).
   Check startup logs for a successful agent manager initialization and no
   `pi binary not found` warning. The server can start with a broken Pi install,
   so a running gallery is insufficient.
5. Default to bring-your-own-key: leave `AGENT_API_KEY` unset, open the agent
   settings in Exhibit, and have the person enter their provider key directly
   there. It is encrypted in the data directory. Do not ask for a key in chat.
   With credentials supplied, ask the agent in Exhibit to change the setup
   artifact's title or text, wait for completion, and verify the change. This
   uses the person's provider account. Without a key, record **agent runtime
   checked; provider interaction pending** and give the exact remaining UI step.
   Instance-paid mode is a separate choice: configure `AGENT_PROVIDER` and an
   agreed spend cap alongside `AGENT_API_KEY`, following
   [deployment §4.1](deployment.md#41-letting-the-instance-supply-the-agent-key-platform-mode).

If browser automation is unavailable, complete terminal checks, give the person
the artifact and the exact browser steps, and label browser rendering/state
verification pending. Never replace unperformed checks with an assumed pass.

## Network access and handoff

For access from another device, use names reachable from that device for both
origins; `localhost` there refers to that device. Set up DNS/TLS and proxy both
origins as described in [deployment §5](deployment.md#5-reverse-proxy--tls).
Use different hostnames for network deployment. The server itself must also be
able to reach `APP_ORIGIN` for its agent tools. Keep backend ports private behind
the proxy, change the initial password before exposing it, and generate a real
`AUTH_TOKEN`; the token is an API credential, not a replacement for browser login.
Self-hosting requires no Clerk account or keys. Existing OIDC/login choices
should be preserved. Recreate/restart the service after changing environment
settings and repeat verification through the public URLs.

Finish with the machine and checkout/revision used, actual gallery and renderer
URLs, login name (no password), config/launcher/data paths or Compose volume,
stop/start/restart/log commands, whether it starts after reboot, verification
results, and any outstanding user action. The source guide's background process
does not survive a reboot; if unattended startup is wanted, configure a user
systemd service on Linux or LaunchAgent on macOS using the same launcher and
record its exact service name and commands. Back up the complete data directory
(including `secret.key`, blobs, and agent sessions) and environment file;
[deployment](deployment.md) covers backup and upgrade considerations.
