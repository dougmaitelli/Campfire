import type { RuntimeStatus, ServerAction } from "../../api/runtime";

export function ServerLifecycleControls({
  status,
  disabled,
  onAction,
  onConsole,
  onFiles,
  onBackups,
}: {
  status: RuntimeStatus | undefined;
  disabled: boolean;
  onAction: (action: ServerAction) => void;
  onConsole: () => void;
  onFiles: () => void;
  onBackups: () => void;
}) {
  const blocked =
    disabled ||
    !status ||
    status.busy ||
    !["running", "stopped", "not_installed", "deleted"].includes(status.state);
  const running = status?.state === "running";
  const install =
    status?.state === "not_installed" || status?.state === "deleted";
  return (
    <>
      <button className="button" disabled={disabled} onClick={onBackups}>
        Backups
      </button>
      <button className="button" disabled={disabled} onClick={onFiles}>
        Files
      </button>
      <button
        className="button button-primary"
        disabled={blocked}
        onClick={() =>
          onAction(install ? "install" : running ? "stop" : "start")
        }
      >
        {install ? "Install" : running ? "Stop" : "Start"}
      </button>
      <select
        aria-label="More server actions"
        className="button"
        value=""
        disabled={blocked}
        onChange={(event) => {
          const action = event.target.value;
          if (action === "restart" || action === "kill") onAction(action);
        }}
      >
        <option value="">More…</option>
        <option value="restart" disabled={install}>
          Restart
        </option>
        <option value="kill" disabled={!running}>
          Kill
        </option>
      </select>
      <button
        className="button"
        disabled={disabled || (install && !status.error)}
        onClick={onConsole}
      >
        Console
      </button>
    </>
  );
}
