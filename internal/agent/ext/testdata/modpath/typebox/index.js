// Minimal TypeBox test double for exhibit.test.ts (see package.json above).
// Implements just the builders exhibit.ts uses; every schema is an opaque
// descriptor object. Never used outside tests — pi resolves the real
// `typebox` package when it loads the extension.
function desc(kind, props) {
	return { kind, ...(props || {}) };
}

export const Type = {
	Object(props, opts) {
		return desc("object", { properties: props || {}, ...(opts || {}) });
	},
	Array(items, opts) {
		return desc("array", { items, ...(opts || {}) });
	},
	String(opts) {
		return desc("string", opts);
	},
	Optional(schema) {
		return { ...schema, optional: true };
	},
};
