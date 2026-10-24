# Agent setup walkthrough results

Validated on **2026-10-24**, using the setup guides in this change and the current
server, Dockerfile, and Compose configuration. Each walkthrough used a separate
temporary checkout and fresh application data. No personal installation or
provider credentials were used.

## Docker Compose

Host: Ubuntu 24.04 LTS, Linux ARM64, Docker Engine 29.1.3, Compose 2.40.3.
The host already had a working Docker engine; the engine/Desktop installation
and first-run GUI steps were reviewed against the linked official instructions,
rather than performed on a fresh host.

- Built the application image from the repository Dockerfile, including embedded
  assets and pinned Pi **1.3.0**. Used that image for the Compose walkthrough
  without rebuilding it a second time.
- Executed the guide's configuration-generation and overlay snippets. Used a
  distinct validation project and free ports 49180/49181, updating both sides
  of mappings, listen addresses, and origins as described. Checked the resolved
  configuration: exactly two port bindings, both on loopback.
- Confirmed `env_file` forwards an additional setting absent from the base
  service's environment list, and the running container can fetch its configured
  `APP_ORIGIN` login page. Confirmed `pi --version` inside the container.
- Changed the seeded password through the CLI using stdin. Verified anonymous
  API rejection and authenticated API access, and imported the guide's exact
  HTML artifact through the API.
- Used the collaborative Chromium browser to sign in, open the artifact, and
  click **Save setup check**. The iframe loaded successfully from the separate
  render origin, and the state API reported `setup-check: Saved`.
- Restarted the service and reloaded the artifact: the browser still showed
  **Saved**. Repeated `up -d` with the existing image and verified unchanged
  configuration, container identity and named data volume. Repeated file
  creation snippets refused to overwrite the existing configuration.

## From source

Built in a clean `ubuntu:24.04` ARM64 container. Installed the guide's host
packages, downloaded architecture-specific Go and Node archives from their
official distribution sites, and verified SHA-256 checksums against published
metadata. Toolchains: **Go 1.25.7**, **Node 22.23.4**, **npm 10.9.9**, GNU Make 4.3,
Python 3 with fontTools/Brotli, and isolated **Pi 1.3.0**.

- Executed the guide's exact pinned Pi install, `go mod download`, and
  `make build` snippets. All asset workspaces built, including font subsetting.
- Executed the exact exclusive environment-file creation and launcher snippets.
  Created the admin through the launcher before starting the server.
- Started with the saved environment, checked local login, rejection of wrong
  passwords and anonymous API access, and authenticated API access.
- Imported the same guide artifact, wrote/read its state through the API, and
  fetched its signed document from the separate render listener. The document
  contained the artifact and its inlined saved state.
- Stopped and restarted through the launcher twice. Artifact/state, configuration
  contents and encryption-secret bytes remained unchanged. Re-running both
  file-creation snippets refused overwrite.
- Verified the startup log enabled the agent runtime. With no provider key, the
  key endpoint reported unconfigured and session creation returned the expected
  **412** credential requirement, rather than a **503** missing-runtime error.

## Additional checks and limits

All setup shell blocks passed `sh -n`, and local documentation links resolved.
Checked Git ignore rules for generated settings/launchers. Built a small Docker
context fixture with sentinel `.env.setup`, `.env.local`, and `.exhibit-setup/`
files and verified none entered the image under the updated `.dockerignore`.

The source browser storage shim was exercised in the Compose walkthrough;
the source-build walkthrough verified HTTP render responses and API persistence.
No external provider interaction was attempted: entering a provider key and
completing a real agent turn remain the explicit credential-dependent steps in
the guide. macOS, x86-64, WSL2, remote DNS/TLS, and startup after a host reboot
were not exercised. Those instructions are adaptation paths, not recorded
platform passes. The example source background launcher intentionally requires
a separate service-manager setup for startup after reboot.
