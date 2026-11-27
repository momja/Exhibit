---
id: Exh-v81w
status: in_progress
deps: []
links: []
created: 2026-11-25T15:35:37Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, experimental]
---
# Artifact inference: send images to complete()

window.exhibit.ai.complete() takes text only. A message's content is a plain string (internal/inference/kinds.go, completeMessage), the payload decoder rejects unknown fields, and the whole request is capped at 48 KB of text. An artifact cannot send a photo, screenshot or scanned page for the model to read; a base64 data: URI passes as ordinary text, which the model cannot interpret and the cap refuses at about 36 KB of image. Tools that read a receipt, describe a picture or extract a table from a screenshot therefore have no path today.

This adds image input to complete(). It is a follow-up to Exh-zue3, which ships text only. decide() stays text and state only: its classifier models take no images, and that is out of scope here.

Not verified yet: that Pi's message format carries image content blocks through the warm worker (internal/inference/worker.go, ext/inference.ts), and that the models offered for AGENT_PROVIDER accept them. The first step is a spike against a real provider with the exhibit-dev instance.

## Design

- Shape. A message's content is a string (as now) or a list of parts: { type: 'text', text } and { type: 'image', mimeType, data } with base64 data. Only user messages may carry images; assistant messages stay text. A string still works unchanged, so existing artifacts are unaffected.
- Allowlist and limits, separate from the 48 KB text cap: MIME types (png, jpeg, webp, gif at most; confirm what providers accept), images per request, bytes per image, bytes in total. Check the bridge's postMessage path and the inference handler's request body limit against the new total. Every refusal is invalid_request before any provider call, with a message that names the limit.
- Capability. The configured chat model may not accept images. Find out whether Pi's model catalog reports input modalities. If it does, startup or available() says whether images work, and a call that sends one to a text-only model is refused with a stable code before any provider call, never as a provider error. If it does not, decide how the operator declares it (for example an INFERENCE_IMAGES setting) and say so in docs/deployment.md. In BYOK mode the answer is per viewer's provider and model.
- available() grows an images flag next to complete and decide, so a tool can hide its upload control when images would be refused.
- Worker and extension. Pass image blocks through the Pi context; each provider call keeps its fresh sessionId. Never log image bytes anywhere, including the job context and error text.
- Repair. runComplete re-sends the context with a repair message when schema output is invalid. Decide whether the repair re-sends the image (a second billed image read) or is skipped for image requests, and test it.
- Metering and belts. The provider's usage already includes image tokens; check that cost_source and the ledger stay correct. Check every belt that sizes a request by text bytes (the rate belts and the shared spend caps) so an image cannot slip under a text-sized estimate.
- Consent. The owner and recipient dialogs and the banner say what a tool may send; images are a different kind of data (photos can carry location metadata). Update the copy and decide whether to strip EXIF server-side or leave that to the tool and say so.
- Agent guidance. internal/agentguidance/inference.tmpl and internal/agent/prompt.go teach the agent about images only when the instance offers them. Teach artifacts to downscale in a canvas before sending, which cuts bytes and tokens.
- Tests. internal/mockllm accepts and records image parts. Table tests for the parser limits, a worker test through the pinned Pi, the repair path, the text-only-model refusal, and a ledger row for an image call. Then one live pass with a vision-capable model through exhibit-dev, recorded in docs/inference.md like the earlier passes.
- Docs. docs/inference.md (payload, limits, error codes, privacy), docs/deployment.md if a new setting is needed, docs/api.md for the payload, architecture.md and the PRD only where they describe what a tool can send.

## Acceptance Criteria

A tool can call complete() with a user message holding text and image parts and get an answer about the image from a vision-capable model. String content behaves exactly as before. Every limit and refusal above is covered by a test and returns a stable code before any provider call. A text-only model is refused with a clear code, and available() reports whether images work. No image bytes appear in logs, the ledger or error messages. A schema repair cannot silently double the image cost. The usage row for an image call is correct and counts against the same caps as any other call. Consent copy and docs say that images leave the viewer's device. decide() is unchanged. One live pass with a real vision model is recorded, and anything not run live is listed as not run.

