import assert from "node:assert/strict";
import test from "node:test";
import { loadWebModule } from "./helpers/load-web-module.mjs";

const { connectConsole } = await loadWebModule(
  new URL("../web/src/components/servers/consoleStream.ts", import.meta.url),
);

class FakeEventSource extends EventTarget {
  CLOSED = 2;
  readyState = 1;
  onerror = null;
  close() {
    this.readyState = this.CLOSED;
  }
  emit(type, value = {}) {
    this.dispatchEvent(new MessageEvent(type, { data: JSON.stringify(value) }));
  }
  disconnect() {
    this.readyState = 0;
    this.onerror?.(new Event("error"));
    // The browser only reconnects automatically if close() was not called.
    if (this.readyState !== this.CLOSED) this.emit("ready");
  }
}

function subscribe(mode) {
  const stream = new FakeEventSource();
  const connections = [];
  const output = [];
  let opened = 0;
  const close = connectConsole(
    {
      url: "/console/stream",
      mode,
      onConnection: (value) => connections.push(value),
      onOutput: (content, replace) => output.push({ content, replace }),
    },
    () => {
      opened++;
      return stream;
    },
  );
  return { stream, connections, output, close, opened };
}

test("completed stopped-server logs close the stream and stay steady", () => {
  const { stream, connections, output } = subscribe("snapshot");
  stream.emit("ready");
  stream.emit("output", { content: "Install Complete\n" });
  stream.emit("ended", { end: true });
  assert.equal(stream.readyState, stream.CLOSED);
  assert.deepEqual(
    connections.map((value) => value.label),
    ["Loading logs…", "Stopped"],
  );
  assert.deepEqual(output, [{ content: "Install Complete\n", replace: true }]);
});

test("failed stopped-server log requests do not automatically reconnect", () => {
  const { stream, connections, output } = subscribe("snapshot");
  stream.disconnect();
  assert.equal(stream.readyState, stream.CLOSED);
  assert.equal(connections.at(-1).label, "Logs unavailable");
  assert.deepEqual(output, []);
});

test("live connection retries preserve output until new data arrives", () => {
  const { stream, output, close } = subscribe("live");
  stream.emit("ready");
  stream.emit("output", { content: "Running\n" });
  stream.disconnect();
  assert.notEqual(stream.readyState, stream.CLOSED);
  assert.deepEqual(output, [{ content: "Running\n", replace: true }]);
  stream.emit("output", { content: "Running again\n" });
  assert.equal(output.at(-1).replace, true);
  close();
  assert.equal(stream.readyState, stream.CLOSED);
});

test("waiting for runtime status does not open a stream", () => {
  const { opened, connections } = subscribe("waiting");
  assert.equal(opened, 0);
  assert.equal(connections.at(-1).label, "Waiting for server status…");
});

test("a new operation can open a live stream after stopped logs finish", () => {
  const idle = subscribe("snapshot");
  idle.stream.emit("ended");
  const active = subscribe("live");
  active.stream.emit("ready");
  active.stream.emit("output", { content: "Installing again\n" });
  assert.equal(active.connections.at(-1).label, "Connected");
  assert.deepEqual(active.output, [
    { content: "Installing again\n", replace: true },
  ]);
  active.close();
});
