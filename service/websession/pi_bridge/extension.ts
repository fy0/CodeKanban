import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

const COMMAND_NAME = "codekanban-navigate";
const MARKER_TYPE = "codekanban.active-leaf.v1";

interface NavigatePayload {
	targetId: string;
	summarize: boolean;
	nonce: string;
}

function decodePayload(raw: string): NavigatePayload {
	const parsed = JSON.parse(Buffer.from(raw.trim(), "base64url").toString("utf8")) as Partial<NavigatePayload>;
	if (
		typeof parsed.targetId !== "string" ||
		parsed.targetId.trim() === "" ||
		typeof parsed.summarize !== "boolean" ||
		typeof parsed.nonce !== "string" ||
		parsed.nonce.trim() === ""
	) {
		throw new Error("invalid CodeKanban navigation payload");
	}
	return {
		targetId: parsed.targetId.trim(),
		summarize: parsed.summarize,
		nonce: parsed.nonce.trim(),
	};
}

export default function (pi: ExtensionAPI) {
	pi.registerCommand(COMMAND_NAME, {
		description: "Internal CodeKanban session-tree navigation bridge",
		handler: async (args, ctx) => {
			const payload = decodePayload(args);
			const result = await ctx.navigateTree(payload.targetId, {
				summarize: payload.summarize,
			});
			if (result.cancelled) {
				throw new Error("Pi session-tree navigation was cancelled");
			}
			pi.appendEntry(MARKER_TYPE, payload);
		},
	});
}
