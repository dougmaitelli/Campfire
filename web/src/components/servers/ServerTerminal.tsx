import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import { ActionGroup } from "../layout/ActionGroup";
import { Button } from "../forms/Button";
import { TextInput } from "../forms/TextInput";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { InlineMessage } from "../status/InlineMessage";
import { useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
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
  const { t } = useTranslation();
  const container = useRef<HTMLDivElement>(null);
  const terminal = useRef<Terminal | null>(null);
  const search = useRef<SearchAddon | null>(null);
  const searchInput = useRef<HTMLInputElement>(null);
  const [searchOpen, setSearchOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [noResults, setNoResults] = useState(false);

  function find(previous = false, incremental = false, value = query) {
    const addon = search.current;
    if (!addon) return;
    if (!value) {
      terminal.current?.clearSelection();
      setNoResults(false);
      return;
    }
    const found = previous
      ? addon.findPrevious(value, { incremental })
      : addon.findNext(value, { incremental });
    setNoResults(!found);
  }

  function openSearch() {
    setSearchOpen(true);
    find(false, true);
    searchInput.current?.focus();
    searchInput.current?.select();
  }

  function closeSearch() {
    setSearchOpen(false);
    terminal.current?.clearSelection();
    terminal.current?.focus();
  }

  useEffect(() => {
    if (searchOpen) searchInput.current?.focus();
  }, [searchOpen]);
  const [connection, setConnection] = useState<{
    label: string;
    tone: StatusTone;
  }>({ label: t("serverTerminal.connecting"), tone: "pending" });
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
        cause instanceof Error
          ? cause.message
          : i18n.t("serverTerminal.unableToSendCommand"),
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
      fontSize: 14,
      lineHeight: 1.5,
      fontFamily: i18n.t(
        "serverTerminal.uiMonospaceSFMonoRegularMenloConsolasMonospace",
      ),
      scrollback: 5000,
      screenReaderMode: true,
      theme: {
        background: colors.getPropertyValue("--console-bg").trim(),
        foreground: colors.getPropertyValue("--text").trim(),
        selectionBackground: colors
          .getPropertyValue("--console-selection")
          .trim(),
        black: colors.getPropertyValue("--surface-hover").trim(),
        red: colors.getPropertyValue("--danger").trim(),
        green: colors.getPropertyValue("--success").trim(),
        yellow: colors.getPropertyValue("--warning").trim(),
        blue: "#7ab7ed",
        magenta: "#c49be8",
        cyan: "#80d4ce",
        white: colors.getPropertyValue("--text").trim(),
        brightBlack: colors.getPropertyValue("--muted").trim(),
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
    const searchAddon = new SearchAddon();
    term.loadAddon(searchAddon);
    search.current = searchAddon;
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
      search.current = null;
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
    <div
      className="server-terminal"
      onKeyDownCapture={(event) => {
        if (
          (event.ctrlKey || event.metaKey) &&
          event.key.toLowerCase() === "f"
        ) {
          event.preventDefault();
          event.stopPropagation();
          openSearch();
        } else if (searchOpen && event.key === "Escape") {
          event.preventDefault();
          event.stopPropagation();
          closeSearch();
        }
      }}
    >
      <div className="server-terminal-window">
        <div className="server-terminal-toolbar">
          {searchOpen && (
            <form
              className="server-terminal-search"
              role="search"
              aria-label={t("serverTerminal.search")}
              onSubmit={(event) => {
                event.preventDefault();
                find();
              }}
            >
              <TextInput
                ref={searchInput}
                aria-label={t("serverTerminal.search")}
                placeholder={t("serverTerminal.search")}
                value={query}
                autoComplete="off"
                spellCheck={false}
                onChange={(event) => {
                  setQuery(event.target.value);
                  find(false, true, event.target.value);
                }}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    find(event.shiftKey);
                  }
                }}
              />
              <Button
                size="small"
                type="button"
                disabled={!query}
                onClick={() => find(true)}
              >
                {t("serverTerminal.previousMatch")}
              </Button>
              <Button size="small" type="submit" disabled={!query}>
                {t("serverTerminal.nextMatch")}
              </Button>
              <Button
                size="small"
                type="button"
                iconOnly
                aria-label={t("serverTerminal.closeSearch")}
                onClick={closeSearch}
              >
                <NavigationIcon name="close" />
              </Button>
              <span className="server-terminal-search-status">
                <span
                  aria-hidden="true"
                  data-label={t("serverTerminal.noResults")}
                />
                <span role="status">
                  {noResults ? t("serverTerminal.noResults") : ""}
                </span>
              </span>
            </form>
          )}
          <span role="status">
            <StatusPill tone={connection.tone}>{connection.label}</StatusPill>
          </span>
          <ActionGroup>
            <Button
              size="small"
              type="button"
              iconOnly
              aria-label={t("serverTerminal.search")}
              title={t("serverTerminal.search")}
              onClick={openSearch}
            >
              <NavigationIcon name="search" />
            </Button>
            <Button
              size="small"
              type="button"
              iconOnly
              aria-label={t("serverTerminal.downloadLogs")}
              title={t(
                "serverTerminal.downloadRetainedConsoleOutputUpTo5000LinesOf",
              )}
              onClick={downloadLogs}
            >
              <NavigationIcon name="download" />
            </Button>
            <Button
              size="small"
              type="button"
              iconOnly
              aria-label={t("serverTerminal.clear")}
              title={t("serverTerminal.clear")}
              onClick={() => terminal.current?.clear()}
            >
              <NavigationIcon name="delete" />
            </Button>
          </ActionGroup>
        </div>
        <div
          ref={container}
          className="server-terminal-screen"
          role="region"
          aria-label={t("serverTerminal.consoleOutput")}
        />
        <form
          className="server-terminal-command"
          onSubmit={(event) => {
            event.preventDefault();
            void send();
          }}
        >
          <div className="server-terminal-input">
            <span aria-hidden="true">{">"}</span>
            <TextInput
              aria-label={t("serverTerminal.command")}
              value={command}
              readOnly={busy}
              maxLength={4096}
              placeholder={
                busy
                  ? t("serverTerminal.sending")
                  : t("serverTerminal.sendCommandPlaceholder")
              }
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
          </div>
          <Button
            type="submit"
            iconOnly
            variant="primary"
            disabled={busy || !command.trim()}
            loading={busy}
            aria-label={t("serverTerminal.sendCommand")}
            title={t("serverTerminal.sendCommand")}
          >
            <NavigationIcon name={busy ? "loading" : "send"} filled={!busy} />
          </Button>
        </form>
      </div>
      {error && <InlineMessage role="alert">{error}</InlineMessage>}
    </div>
  );
}
