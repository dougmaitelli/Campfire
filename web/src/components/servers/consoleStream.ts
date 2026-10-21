import i18n from "../../i18n";
import type { StatusTone } from "../status/StatusPill";
import { ConsoleTimestamps } from "./consoleTimestamps";

export type ConsoleStreamMode = "waiting" | "snapshot" | "live";

export function connectConsole(
  {
    url,
    mode,
    onConnection,
    onOutput,
  }: {
    url: string;
    mode: ConsoleStreamMode;
    onConnection: (state: { label: string; tone: StatusTone }) => void;
    onOutput: (content: string, replace: boolean) => void;
  },
  createStream: (url: string) => EventSource = (url) => new EventSource(url),
): () => void {
  if (mode === "waiting") {
    onConnection({
      label: i18n.t("consoleStream.waitingForServerStatus"),
      tone: "neutral",
    });
    return () => {};
  }
  const live = mode === "live";
  onConnection({
    label: live
      ? i18n.t("consoleStream.connecting")
      : i18n.t("consoleStream.loadingLogs"),
    tone: "pending",
  });
  const stream = createStream(url);
  let ended = false;
  let replaceOutput = true;
  const timestamps = new ConsoleTimestamps();
  const flush = () => {
    const pending = timestamps.flush();
    if (pending) onOutput(pending, false);
  };
  stream.addEventListener("ready", () => {
    ended = false;
    replaceOutput = true;
    if (live)
      onConnection({ label: i18n.t("consoleStream.live"), tone: "positive" });
  });
  stream.addEventListener("output", (event: MessageEvent<string>) => {
    try {
      const value: unknown = JSON.parse(event.data);
      if (
        typeof value === "object" &&
        value !== null &&
        "content" in value &&
        typeof value.content === "string" &&
        value.content.length > 0
      ) {
        if (replaceOutput) timestamps.reset();
        onOutput(timestamps.format(value.content), replaceOutput);
        replaceOutput = false;
      }
    } catch {
      onConnection({ label: "Invalid console output.", tone: "negative" });
    }
  });
  stream.addEventListener("ended", () => {
    flush();
    ended = true;
    if (!live) stream.close();
    onConnection({
      label: live ? "Waiting for server output…" : "Stopped",
      tone: "neutral",
    });
  });
  stream.addEventListener("denied", () => {
    flush();
    ended = true;
    stream.close();
    onConnection({ label: "Console access ended.", tone: "negative" });
  });
  stream.onerror = () => {
    flush();
    if (ended) return;
    if (!live) {
      stream.close();
      onConnection({
        label: i18n.t("consoleStream.logsUnavailable"),
        tone: "negative",
      });
      return;
    }
    onConnection({
      label:
        stream.readyState === stream.CLOSED
          ? i18n.t("consoleStream.consoleUnavailable")
          : i18n.t("consoleStream.connectionLostReconnecting"),
      tone: "pending",
    });
  };
  return () => stream.close();
}
