import { spawn } from "node:child_process";
import { watch } from "node:fs";
import { mkdir, rm } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const mock = process.argv.includes("--mock");
const backendDirectory = join(root, "backend");
const database =
  process.env.CAMPFIRE_DATABASE ?? join(root, "data", "panel.db");
const binary = join(root, "bin", mock ? "campfire-mock" : "campfire-dev");
const children = new Set();
let backend;
let watcher;
let rebuildTimer;
let building = false;
let dirty = false;
let stopping = false;

function start(command, args, cwd = root) {
  const child = spawn(command, args, {
    cwd,
    stdio: "inherit",
    env: { ...process.env, VITE_CAMPFIRE_MOCK: mock ? "true" : "false" },
    detached: process.platform !== "win32",
  });
  children.add(child);
  child.once("close", () => children.delete(child));
  child.once("error", (error) => {
    console.error("[dev]", error.message);
  });
  return child;
}

function completed(child) {
  return new Promise((resolve) => {
    child.once("error", () => resolve(1));
    child.once("close", (code) => resolve(code ?? 1));
  });
}

function signal(child, name) {
  if (!child.pid) return;
  try {
    if (process.platform === "win32") child.kill(name);
    else process.kill(-child.pid, name);
  } catch (error) {
    if (error.code !== "ESRCH") console.error("[dev]", error.message);
  }
}

async function stop(child) {
  if (!child || !children.has(child)) return;
  const closed = completed(child);
  signal(child, "SIGTERM");
  const exited = await Promise.race([
    closed.then(() => true),
    delay(2000, undefined, { ref: false }).then(() => false),
  ]);
  if (!exited) {
    signal(child, "SIGKILL");
    await closed;
  }
}

async function shutdown(code) {
  if (stopping) return;
  stopping = true;
  process.exitCode = code;
  clearTimeout(rebuildTimer);
  watcher?.close();
  await Promise.all([...children].map(stop));
  console.log("[dev] Stopped backend and UI.");
}

async function rebuild() {
  dirty = true;
  if (building || stopping) return;
  building = true;
  try {
    while (dirty && !stopping) {
      dirty = false;
      console.log("[dev] Building backend…");
      const build = start(
        "go",
        [
          "build",
          ...(mock ? ["-gcflags=all=-N -l"] : []),
          "-buildvcs=false",
          "-o",
          binary,
          mock ? "./cmd/campfire-mock" : "./cmd/campfire",
        ],
        backendDirectory,
      );
      const code = await completed(build);
      if (stopping) return;
      if (dirty) continue;
      if (code !== 0) {
        console.error("[dev] Build failed; fix the errors and save to retry.");
        continue;
      }
      const previous = backend;
      backend = undefined;
      await stop(previous);
      if (stopping) return;
      if (dirty) continue;
      console.log("[dev] Starting backend at http://127.0.0.1:8080");
      const running = start(binary, mock ? [] : ["--database", database]);
      backend = running;
      void completed(running).then((exitCode) => {
        if (!stopping && backend === running) {
          console.error("[dev] Backend exited unexpectedly.");
          return shutdown(exitCode || 1);
        }
      });
    }
  } finally {
    building = false;
  }
}

function scheduleRebuild(_event, filename) {
  if (!filename) return;
  const path = filename.toString();
  if (!path.endsWith(".go") && path !== "go.mod" && path !== "go.sum") return;
  // Mark changes immediately so an in-progress build cannot start stale code.
  dirty = true;
  clearTimeout(rebuildTimer);
  rebuildTimer = setTimeout(() => {
    void rebuild().catch((error) => {
      console.error("[dev]", error.message);
      void shutdown(1);
    });
  }, 200);
}

process.on("SIGINT", () => {
  void shutdown(0);
});
process.on("SIGTERM", () => {
  void shutdown(0);
});

try {
  await mkdir(join(root, "bin"), { recursive: true });
  if (mock) {
    // Reset once per dev session; watcher-triggered rebuilds reuse this state.
    await rm(join(root, "data", "mock"), { recursive: true, force: true });
    console.log("[dev] Started a fresh mock session.");
  }
  if (!stopping) {
    watcher = watch(backendDirectory, { recursive: true }, scheduleRebuild);
    watcher.on("error", (error) => {
      console.error("[dev] Watcher failed:", error.message);
      void shutdown(1);
    });
    const ui = start(
      process.execPath,
      [join(root, "web", "node_modules", "vite", "bin", "vite.js")],
      join(root, "web"),
    );
    void completed(ui).then((code) => {
      if (!stopping) {
        console.error("[dev] UI exited.");
        return shutdown(code || 1);
      }
    });
    await rebuild();
  }
} catch (error) {
  console.error("[dev]", error.message);
  await shutdown(1);
}
