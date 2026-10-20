import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
  const installing =
    status?.busy === true &&
    ["install", "reinstall", "update"].includes(status.operation ?? "");
  const canInterrupt = !disabled && installing;
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
        disabled={!canInterrupt && (blocked || install)}
        onClick={() => onAction(running || installing ? "stop" : "start")}
      >
        {running || installing ? t("common.stop") : t("common.start")}
      </Button>
      <Button
        disabled={blocked || !running}
        onClick={() => onAction("restart")}
      >
        {t("common.restart")}
      </Button>
      <Button
        variant="danger"
        disabled={!canInterrupt && (blocked || !running)}
        onClick={() => onAction("kill")}
      >
        {t("common.kill")}
      </Button>
      {(install || canConfigure) && (
        <Button
          variant={install ? "primary" : "warning"}
          disabled={blocked || running}
          onClick={() => onAction(install ? "install" : "reinstall")}
        >
          {install ? t("common.install") : t("common.reinstall")}
        </Button>
      )}
    </>
  );
}
