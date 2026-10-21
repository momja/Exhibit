// --import entry point registering the typebox resolve hook for tests.
import { register } from "node:module";

register("./resolve-typebox.mjs", import.meta.url);
