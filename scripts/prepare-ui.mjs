import { cp, mkdir, readFile, rm } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const source = join(root, "web", "dist");
const destination = join(root, "backend", "internal", "webui", "dist");

// Only generated build artifacts are copied; source files are never rewritten.
await readFile(join(source, "index.html"));
await rm(destination, { recursive: true, force: true });
await mkdir(dirname(destination), { recursive: true });
await cp(source, destination, { recursive: true, errorOnExist: true });
