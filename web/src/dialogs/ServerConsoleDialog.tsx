import { useEffect, useRef, useState } from "react";
import type { Server } from "../api/servers";
import { sendConsole, serverConsole } from "../api/runtime";
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
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function poll() {
      try {
        const status = await serverConsole(
          server.id,
          AbortSignal.any([controller.signal, AbortSignal.timeout(8000)]),
        );
        if (!controller.signal.aborted) {
          setLogs(status.logs ?? "");
          setError(null);
        }
      } catch (cause) {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error ? cause.message : "Unable to read console.",
          );
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => {
          void poll();
        }, 2000);
    }
    void poll();
    return () => {
      controller.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
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
      description="Live console output refreshes every two seconds. Shows the latest 200 lines."
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
        {error && (
          <p className="error-message" role="alert">
            {error}
          </p>
        )}
        <pre
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
