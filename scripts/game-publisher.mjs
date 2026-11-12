// Run inside an opted-in game container, or supply its host-side socket path.
// node game-publisher.mjs [state|event] [socket-path]
import http from "node:http";
import { randomUUID } from "node:crypto";

const [kind = "state", socketPath = process.env.CAMPFIRE_GAME_SOCKET] =
  process.argv.slice(2);

if (!socketPath || !["state", "event"].includes(kind)) {
  throw new Error(
    "Usage: node game-publisher.mjs [state|event] [socket-path]; defaults to CAMPFIRE_GAME_SOCKET",
  );
}

function send(path, method, publication) {
  return new Promise((resolve, reject) => {
    const body = JSON.stringify(publication);
    const request = http.request(
      {
        socketPath,
        path,
        method,
        headers: {
          "Content-Type": "application/json",
          "Content-Length": Buffer.byteLength(body),
        },
        timeout: 12000,
      },
      (response) => {
        response.setEncoding("utf8");
        let detail = "";
        response.on("data", (chunk) => {
          detail += chunk;
        });
        response.on("error", reject);
        response.on("end", () => {
          if (response.statusCode !== 204 && response.statusCode !== 202) {
            reject(
              new Error(`Request failed (${response.statusCode}): ${detail}`),
            );
            return;
          }
          resolve(response.statusCode);
        });
      },
    );
    request.on("timeout", () =>
      request.destroy(new Error("Request timed out")),
    );
    request.on("error", reject);
    request.end(body);
  });
}

// The manifest is stable across retries and process restarts. Increase revision
// when changing definitions; data publications never repeat these labels.
await send("/v1/registration", "PUT", {
  version: 1,
  registration: {
    revision: 1,
    state_fields: { game_mode: "Game mode" },
    event_fields: {
      "game.announcement": {
        world: "World name",
        message: "Message",
        mod_version: "Mod version",
      },
    },
  },
});

const publication = {
  version: 1,
  ...(kind === "state"
    ? { state: { players: 3, capacity: 20, fields: { game_mode: "survival" } } }
    : {
        event: {
          id: randomUUID(),
          type: "game.announcement",
          occurred_at: new Date().toISOString(),
          title: "Hello from the example game integration",
          data: { message: "Published through Spark" },
        },
      }),
};

const status = await send("/v1/publish", "POST", publication);
console.log(
  status === 202
    ? "Spark durably queued the event for delivery."
    : `Campfire acknowledged ${kind} publication.`,
);
