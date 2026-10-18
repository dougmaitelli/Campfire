import type { StatusTone } from "../status/StatusPill";

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
    onConnection({ label: "Waiting for server status…", tone: "neutral" });
    return () => {};
  }
  const live = mode === "live";
  onConnection({
    label: live ? "Connecting…" : "Loading logs…",
    tone: "pending",
  });
  const stream = createStream(url);
  let ended = false;
  let replaceOutput = true;
  stream.addEventListener("ready", () => {
    ended = false;
    replaceOutput = true;
    if (live) onConnection({ label: "Connected", tone: "positive" });
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
        onOutput(value.content, replaceOutput);
        replaceOutput = false;
      }
    } catch {
      onConnection({ label: "Invalid console output.", tone: "negative" });
    }
  });
  stream.addEventListener("ended", () => {
    ended = true;
    if (!live) stream.close();
    onConnection({
      label: live ? "Waiting for server output…" : "Stopped",
      tone: "neutral",
    });
  });
  stream.addEventListener("denied", () => {
    ended = true;
    stream.close();
    onConnection({ label: "Console access ended.", tone: "negative" });
  });
  stream.onerror = () => {
    if (ended) return;
    if (!live) {
      stream.close();
      onConnection({ label: "Logs unavailable", tone: "negative" });
      return;
    }
    onConnection({
      label:
        stream.readyState === stream.CLOSED
          ? "Console unavailable"
          : "Connection lost; reconnecting…",
      tone: "pending",
    });
  };
  return () => stream.close();
}
