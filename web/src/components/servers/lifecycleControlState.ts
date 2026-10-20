import type { RuntimeStatus } from "../../api/runtime";

export function lifecycleControlState(
  status: RuntimeStatus | undefined,
  disabled: boolean,
) {
  const installing =
    status?.busy === true &&
    ["install", "reinstall", "update"].includes(status.operation ?? "");
  return {
    installing,
    canInterrupt: !disabled && installing,
    blocked:
      disabled ||
      !status ||
      status.busy ||
      ![
        "running",
        "restarting",
        "stopped",
        "not_installed",
        "deleted",
      ].includes(status.state),
    running: status?.state === "running" || status?.state === "restarting",
    install: status?.state === "not_installed" || status?.state === "deleted",
  };
}
