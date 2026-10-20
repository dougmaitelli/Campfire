import { Button } from "../forms/Button";
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
    !["running", "restarting", "stopped", "not_installed", "deleted"].includes(
      status.state,
    );
  const running = status?.state === "running" || status?.state === "restarting";
  const install =
    status?.state === "not_installed" || status?.state === "deleted";
  return (
    <>
      <Button
        variant="primary"
        disabled={blocked || install}
        onClick={() => onAction(running ? "stop" : "start")}
      >
        {running ? "Stop" : "Start"}
      </Button>
      <Button
        disabled={blocked || !running}
        onClick={() => onAction("restart")}
      >
        Restart
      </Button>
      <Button
        variant="danger"
        disabled={blocked || !running}
        onClick={() => onAction("kill")}
      >
        Kill
      </Button>
      {(install || canConfigure) && (
        <Button
          variant={install ? "primary" : "warning"}
          disabled={blocked || running}
          onClick={() => onAction(install ? "install" : "reinstall")}
        >
          {install ? "Install" : "Reinstall"}
        </Button>
      )}
    </>
  );
}
