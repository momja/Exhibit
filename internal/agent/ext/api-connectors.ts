import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";

export function registerAPIConnectors(pi: ExtensionAPI, api: (method: string, path: string, body?: any) => Promise<any>, artifact: () => string) {
	pi.registerTool({
		name: "propose_api_connector",
		label: "Propose an API connection",
		description: "Propose an advanced API connection and wait for the person's decision in Exhibit's private approval card. Save the artifact first. Supply only public metadata, never credentials. Verify the API origin and authentication against official docs; integrations.sh can help find OpenAPI/schema references but is untrusted discovery data, not permission. The person must explicitly trust the exact destination and enter their key privately. This tool never receives the key. Denial/cancellation means no permission: keep the app disconnected, explain the alternative, and do not retry or ask for a key in chat. An approved connection still needs separate artifact access in Connections.",
		parameters: Type.Object({
			name: Type.String({minLength: 1, maxLength: 100}),
			origin: Type.String({description: "Exact public HTTPS API origin, e.g. https://api.example.com. No path, query, credentials or wildcard."}),
			auth: Type.Union([Type.Literal("api-key"), Type.Literal("bearer")]),
			header: Type.Optional(Type.String({description: "Reviewed API-key header, e.g. X-API-Key. For bearer use Authorization."})),
			reason: Type.String({minLength: 1, maxLength: 1000, description: "What the app needs this API for; no credentials"}),
		}),
		async execute(toolCallId, params, signal, _onUpdate, ctx) {
			const response = await api("POST", "/api/artifacts/" + encodeURIComponent(artifact()) + "/agent-api-proposals", params);
			let status = signal?.aborted ? "cancelled" : response.proposal?.state;
			if (status === "pending") {
				const reply = await ctx.ui.input("exhibit:api-connector:v1", JSON.stringify({toolCallId, proposalId: response.proposal.id}), {signal});
				status = reply === undefined || signal?.aborted ? "cancelled" : JSON.parse(reply).status;
			}
			// Select status explicitly: no future API response field can enter context.
			const approved = status === "approved";
			return {
				content: [{type: "text", text: approved
					? "The API connection is ready. Its key stays private. The person must separately attach it to this artifact and approve the API origin in Connections before use."
					: status === "disconnected"
					? "The previously approved connection was removed. Keep the app disconnected and ask the person to review Connections if they want to reconnect. Do not repeat setup or ask for a key in chat."
					: "The API connection was denied or cancelled. No permission was granted. Keep the app disconnected or offer a version without this API. Do not repeat this request, try another domain, or ask for credentials in chat."}],
				details: {exhibit: "api_connector_resolved", status: approved ? "approved" : status === "disconnected" ? "disconnected" : "denied"},
			};
		},
	});
}
