import { useEffect, useRef, useState } from "react";
import type { Server } from "../api/servers";
import { sendConsole } from "../api/runtime";
import { Drawer } from "../components/layout/Drawer";

export function ServerConsoleDialog({
  server,
  onClose,
}: {
  server: Server;
  onClose: () => void;
}) {
  const [logs, setLogs] = useState("Loading console…");
  const [command, setCommand] = useState("");
  const [busy, setBusy] = useState(false);
  const pending = useRef(false);
  const output = useRef<HTMLPreElement>(null);
  const following = useRef(true);
  const [error, setError] = useState<string | null>(null);
  const [connection, setConnection] = useState("Connecting…");
  useEffect(() => {
    if (following.current && output.current)
      output.current.scrollTop = output.current.scrollHeight;
  }, [logs]);
  useEffect(() => {
    const stream = new EventSource(
      `/api/v1/servers/${encodeURIComponent(server.id)}/console/stream`,
    );
    stream.addEventListener("ready", () => {
      setLogs("");
      setConnection("Connected");
    });
    stream.addEventListener("output", (event: MessageEvent<string>) => {
      try {
        const value: unknown = JSON.parse(event.data);
        if (
          typeof value !== "object" ||
          value === null ||
          !("content" in value) ||
          typeof value.content !== "string"
        )
          return;
        const content = value.content;
        setLogs((previous) => (previous + content).slice(-262144));
      } catch {
        setConnection("Invalid console output. Close and reopen to retry.");
        stream.close();
      }
    });
    stream.addEventListener("ended", () =>
      setConnection("Console ended; reconnecting…"),
    );
    stream.addEventListener("denied", () => {
      setConnection("Console access ended.");
      stream.close();
    });
    stream.onerror = () =>
      setConnection(
        stream.readyState === EventSource.CLOSED
          ? "Console unavailable. Close and reopen to retry."
          : "Connection lost; reconnecting…",
      );
    return () => stream.close();
  }, [server.id]);
  async function send() {
    if (pending.current || !command.trim()) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await sendConsole(server.id, command);
      setCommand("");
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to send command.",
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={`${server.name} console`}
      description="Continuous console output. Reconnects with recent history; keeps the latest 256K characters in this view."
      busy={busy}
      onClose={onClose}
      footerLeft={
        <button className="button" disabled={busy} onClick={onClose}>
          Close
        </button>
      }
      footerRight={
        <button
          className="button button-primary"
          disabled={busy || !command.trim()}
          onClick={() => {
            void send();
          }}
        >
          {busy ? "Sending…" : "Send command"}
        </button>
      }
    >
      <div className="drawer-content">
        <p className="field-help" role="status">
          {connection}
        </p>
        {error && (
          <p className="error-message" role="alert">
            {error}
          </p>
        )}
        <pre
          ref={output}
          onScroll={() => {
            const element = output.current;
            if (element)
              following.current =
                element.scrollHeight -
                  element.scrollTop -
                  element.clientHeight <
                32;
          }}
          className="server-console-output"
          tabIndex={0}
          aria-label="Console output"
        >
          {logs}
        </pre>
        <label htmlFor="console-command">Command</label>
        <input
          id="console-command"
          value={command}
          disabled={busy}
          maxLength={4096}
          autoComplete="off"
          spellCheck={false}
          onChange={(event) => setCommand(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              void send();
            }
          }}
        />
      </div>
    </Drawer>
  );
}
