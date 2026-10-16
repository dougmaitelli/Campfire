import { useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { StatusPill } from "../status/StatusPill";
import { sendConsole } from "../../api/runtime";
import "@xterm/xterm/css/xterm.css";
import "./server-terminal.css";

export function ServerTerminal({
  serverID,
  onBusyChange,
}: {
  serverID: string;
  onBusyChange: (busy: boolean) => void;
}) {
  const container = useRef<HTMLDivElement>(null);
  const terminal = useRef<Terminal | null>(null);
  const [connection, setConnection] = useState("Connecting…");
  const [connected, setConnected] = useState(false);
  const [revision, setRevision] = useState(0);
  const [command, setCommand] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  const history = useRef<string[]>([]);
  const historyIndex = useRef(0);
  const draft = useRef("");
  async function send() {
    if (pending.current || !command.trim()) return;
    pending.current = true;
    setBusy(true);
    onBusyChange(true);
    setError(null);
    try {
      await sendConsole(serverID, command);
      if (history.current.at(-1) !== command) history.current.push(command);
      history.current = history.current.slice(-100);
      historyIndex.current = history.current.length;
      draft.current = "";
      setCommand("");
      terminal.current?.scrollToBottom();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to send command.",
      );
    } finally {
      pending.current = false;
      setBusy(false);
      onBusyChange(false);
    }
  }
  useEffect(() => {
    if (!container.current) return;
    const colors = getComputedStyle(document.documentElement);
    const term = new Terminal({
      disableStdin: true,
      convertEol: true,
      cursorBlink: false,
      cursorInactiveStyle: "none",
      fontSize: 13,
      fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
      scrollback: 5000,
      screenReaderMode: true,
      theme: {
        background: colors.getPropertyValue("--bg").trim(),
        foreground: colors.getPropertyValue("--text").trim(),
        selectionBackground: "#efa45b40",
        black: "#282c24",
        red: "#f87171",
        green: "#86c98a",
        yellow: "#e8c66b",
        blue: "#7ab7ed",
        magenta: "#c49be8",
        cyan: "#80d4ce",
        white: "#eeeadd",
        brightBlack: "#b0b5a3",
        brightRed: "#fca5a5",
        brightGreen: "#a7dfab",
        brightYellow: "#f4da92",
        brightBlue: "#a4cff4",
        brightMagenta: "#d9b9f1",
        brightCyan: "#a4e5e0",
        brightWhite: "#ffffff",
      },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(container.current);
    terminal.current = term;
    let frame = 0;
    const observer = new ResizeObserver(() => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => fit.fit());
    });
    observer.observe(container.current);
    fit.fit();
    const stream = new EventSource(
      `/api/v1/servers/${encodeURIComponent(serverID)}/console/stream`,
    );
    stream.addEventListener("ready", () => {
      term.reset();
      setConnected(true);
      setConnection("Connected");
    });
    stream.addEventListener("output", (event: MessageEvent<string>) => {
      try {
        const value: unknown = JSON.parse(event.data);
        if (
          typeof value === "object" &&
          value !== null &&
          "content" in value &&
          typeof value.content === "string"
        )
          term.write(value.content);
      } catch {
        setConnected(false);
        setConnection("Invalid console output. Reconnect to retry.");
        stream.close();
      }
    });
    stream.addEventListener("ended", () => {
      setConnected(false);
      setConnection("Console ended; reconnecting…");
    });
    stream.addEventListener("denied", () => {
      setConnected(false);
      setConnection("Console access ended.");
      stream.close();
    });
    stream.onerror = () => {
      setConnected(false);
      setConnection(
        stream.readyState === EventSource.CLOSED
          ? "Console unavailable"
          : "Connection lost; reconnecting…",
      );
    };
    return () => {
      stream.close();
      observer.disconnect();
      cancelAnimationFrame(frame);
      terminal.current = null;
      term.dispose();
    };
  }, [serverID, revision]);
  return (
    <div className="server-terminal">
      <div className="server-terminal-toolbar">
        <span role="status">
          <StatusPill
            status={
              connected
                ? "connected"
                : connection === "Connecting…"
                  ? "pending"
                  : "disconnected"
            }
          >
            {connection}
          </StatusPill>
        </span>
        <div className="row-actions">
          <button
            type="button"
            className="button"
            onClick={() => terminal.current?.clear()}
          >
            Clear
          </button>
          {!connected && (
            <button
              type="button"
              className="button"
              onClick={() => {
                setConnection("Connecting…");
                setRevision((value) => value + 1);
              }}
            >
              Reconnect
            </button>
          )}
        </div>
      </div>
      <div className="server-terminal-window">
        <div
          ref={container}
          className="server-terminal-screen"
          role="region"
          aria-label="Console output"
        />
        <form
          className="server-terminal-command"
          onSubmit={(event) => {
            event.preventDefault();
            void send();
          }}
        >
          <span aria-hidden="true">&gt;</span>
          <input
            aria-label="Command"
            value={command}
            readOnly={busy}
            maxLength={4096}
            placeholder={busy ? "Sending…" : "Type a command and press Enter"}
            autoComplete="off"
            autoCapitalize="off"
            spellCheck={false}
            onChange={(event) => {
              setCommand(event.target.value);
              historyIndex.current = history.current.length;
              draft.current = event.target.value;
            }}
            onKeyDown={(event) => {
              if (
                busy ||
                (event.key !== "ArrowUp" && event.key !== "ArrowDown")
              )
                return;
              event.preventDefault();
              if (historyIndex.current === history.current.length)
                draft.current = command;
              historyIndex.current = Math.max(
                0,
                Math.min(
                  history.current.length,
                  historyIndex.current + (event.key === "ArrowUp" ? -1 : 1),
                ),
              );
              setCommand(
                history.current[historyIndex.current] ?? draft.current,
              );
            }}
          />
        </form>
      </div>
      {error && (
        <p className="error-message" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
