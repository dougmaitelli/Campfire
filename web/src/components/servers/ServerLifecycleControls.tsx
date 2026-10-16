import type { RuntimeStatus, ServerAction } from "../../api/runtime";

export function ServerLifecycleControls({
  status,
  disabled,
  canConfigure,
  onAction,
}: {
  status: RuntimeStatus | undefined;
  disabled: boolean;
  canConfigure: boolean;
  onAction: (action: ServerAction) => void;
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
      <button
        className="button button-primary"
        disabled={blocked}
        onClick={() =>
          onAction(install ? "install" : running ? "stop" : "start")
        }
      >
        {install ? "Install" : running ? "Stop" : "Start"}
      </button>
      <button
        className="button"
        disabled={blocked || !running}
        onClick={() => onAction("restart")}
      >
        Restart
      </button>
      <button
        className="button button-danger"
        disabled={blocked || !running}
        onClick={() => onAction("kill")}
      >
        Kill
      </button>
      {canConfigure && (
        <>
          <button
            className="button"
            disabled={blocked || running}
            onClick={() => onAction("update")}
          >
            Update game files
          </button>
          <button
            className="button"
            disabled={blocked || running}
            onClick={() => onAction("reinstall")}
          >
            Reinstall (keep files)
          </button>
        </>
      )}
    </>
  );
}
