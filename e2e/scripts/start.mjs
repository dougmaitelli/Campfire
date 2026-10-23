import { spawn } from "node:child_process";
import path from "node:path";

const root = path.resolve(import.meta.dirname, "../..");
const env = Object.fromEntries(
  Object.entries(process.env).filter(([key]) => !key.startsWith("CAMPFIRE_")),
);
let child;
let stopping = false;
for (const signal of ["SIGTERM", "SIGINT"]) {
  process.on(signal, () => {
    stopping = true;
    child?.kill(signal);
  });
}

async function run(command, args, cwd) {
  if (stopping) process.exit(0);
  child = spawn(command, args, { cwd, env, stdio: "inherit" });
  const code = await new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code) => resolve(code ?? 1));
  });
  if (code !== 0) process.exit(stopping ? 0 : code);
}

await run("pnpm", ["run", "build"], path.join(root, "web"));
await run(
  "go",
  [
    "build",
    "-buildvcs=false",
    "-tags=e2e",
    "-ldflags=-X campfire/internal/shared/buildinfo.Version=e2e",
    "-o",
    "../.cache/e2e/server",
    "./e2e",
  ],
  path.join(root, "backend"),
);
await run(path.join(root, ".cache/e2e/server"), [], root);
