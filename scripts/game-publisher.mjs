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

const body = JSON.stringify(publication);
const request = http.request(
  {
    socketPath,
    path: "/v1/publish",
    method: "POST",
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
    response.on("end", () => {
      if (response.statusCode !== 204) {
        console.error(`Publication failed (${response.statusCode}): ${detail}`);
        process.exitCode = 1;
        return;
      }
      console.log(`Campfire acknowledged ${kind} publication.`);
    });
  },
);
request.on("timeout", () =>
  request.destroy(new Error("Publication timed out")),
);
request.on("error", (error) => {
  console.error(error.message);
  process.exitCode = 1;
});
request.end(body);
