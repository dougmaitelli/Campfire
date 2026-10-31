import { RuntimeState, ServerAction } from "../../contracts/domains";
import type { RuntimeStatus } from "../../api/runtime";

export function lifecycleControlState(
  status: RuntimeStatus | undefined,
  disabled: boolean,
) {
  const installing =
    status?.busy === true &&
    [ServerAction.Install, ServerAction.Reinstall, ServerAction.Update].some(
      (value) => value === status.operation,
    );
  return {
    installing,
    canInterrupt: !disabled && installing,
    blocked:
      disabled ||
      !status ||
      status.busy ||
      ![
        RuntimeState.Running,
        RuntimeState.Restarting,
        RuntimeState.Stopped,
        RuntimeState.NotInstalled,
        RuntimeState.Deleted,
      ].some((value) => value === status.state),
    running:
      status?.state === RuntimeState.Running || status?.state === "restarting",
    install:
      status?.state === RuntimeState.NotInstalled ||
      status?.state === "deleted",
  };
}
