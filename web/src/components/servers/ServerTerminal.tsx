import { Button } from "../forms/Button";
import { TextInput } from "../forms/TextInput";
import { InlineMessage } from "../status/InlineMessage";
import { useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { StatusPill, type StatusTone } from "../status/StatusPill";
import { sendConsole } from "../../api/runtime";
import { connectConsole, type ConsoleStreamMode } from "./consoleStream";
import "@xterm/xterm/css/xterm.css";
import "./server-terminal.css";

export function ServerTerminal({
  serverID,
  streamKey,
  streamMode,
  onBusyChange,
}: {
  serverID: string;
  streamKey: string;
  streamMode: ConsoleStreamMode;
  onBusyChange: (busy: boolean) => void;
}) {
  const container = useRef<HTMLDivElement>(null);
  const terminal = useRef<Terminal | null>(null);
  const [connection, setConnection] = useState<{
    label: string;
    tone: StatusTone;
  }>({ label: "Connecting…", tone: "pending" });
  const [command, setCommand] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  const history = useRef<string[]>([]);
  const historyIndex = useRef(0);
  const draft = useRef("");
  function downloadLogs() {
    const buffer = terminal.current?.buffer.active;
    if (!buffer) return;
    let content = "";
    for (let index = 0; index < buffer.length; index++) {
      const line = buffer.getLine(index);
      if (!line) continue;
      if (index > 0 && !line.isWrapped) content += "\n";
      content += line.translateToString(true);
    }
    const url = URL.createObjectURL(
      new Blob([content.trimEnd() + "\n"], {
        type: "text/plain;charset=utf-8",
      }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = `console-${serverID}-${new Date().toISOString().replace(/[:.]/g, "-")}.log`;
    document.body.append(link);
    link.click();
    link.remove();
    window.setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
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
    return () => {
      observer.disconnect();
      cancelAnimationFrame(frame);
      terminal.current = null;
      term.dispose();
    };
  }, [serverID]);
  useEffect(() => {
    const term = terminal.current;
    if (!term) return;
    return connectConsole({
      url: `/api/v1/servers/${encodeURIComponent(serverID)}/console/stream`,
      mode: streamMode,
      onConnection: setConnection,
      onOutput: (content, replace) => {
        term.write(`${replace ? "\x1b[H\x1b[2J" : ""}${content}`);
      },
    });
  }, [serverID, streamKey, streamMode]);
  return (
    <div className="server-terminal">
      <div className="server-terminal-toolbar">
        <span role="status">
          <StatusPill tone={connection.tone}>{connection.label}</StatusPill>
        </span>
        <div className="row-actions">
          <Button
            type="button"

            title="Download retained console output (up to 5,000 lines of scrollback)."
            onClick={downloadLogs}
          >
            Download logs
          </Button>
          <Button
            type="button"

            onClick={() => terminal.current?.clear()}
          >
            Clear
          </Button>
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
          <TextInput
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
      {error && <InlineMessage role="alert">{error}</InlineMessage>}
    </div>
  );
}
