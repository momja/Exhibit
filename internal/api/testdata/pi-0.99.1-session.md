`pi-0.99.1-session.jsonl` is Pi's unmodified session file from a real 0.99.1
sidecar running the Exhibit extension against `internal/mockllm`. It contains
one settled turn: “make the button purple”, `get_artifact`, `write_artifact`,
and the assistant's reply. The artifact and credentials belonged to an isolated
test harness; no provider credential is needed to replay it.

Captured for Exh-xco9 before upgrading the Docker pin to 1.3.0. Keep this
fixture from the old release: generating it with the current Pi would lose the
cross-version resume regression coverage in `TestPiResumesAConversationPersistedBy0991`.
