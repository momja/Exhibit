// Resolve hook mapping bare `typebox` to the test double (see modpath/).
// Registered via --import of register-hooks.mjs, so both direct `node --test`
// runs and the child processes node spawns per test file resolve it.
export async function resolve(specifier, context, nextResolve) {
	if (specifier === "typebox") {
		return nextResolve(new URL("./modpath/typebox/index.js", import.meta.url).href, context);
	}
	return nextResolve(specifier, context);
}
