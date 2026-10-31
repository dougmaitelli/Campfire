import { RuntimeState, type LifecycleAction } from "../../contracts/domains";
import { useTranslation } from "react-i18next";
import { Button } from "../forms/Button";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { type RuntimeStatus, ServerAction } from "../../api/runtime";
import { lifecycleControlState } from "./lifecycleControlState";

export function ServerLifecycleControls({
  status,
  disabled,
  canConfigure,
  onAction,
}: {
  status: RuntimeStatus | undefined;
  disabled: boolean;
  canConfigure: boolean;
  onAction: (action: LifecycleAction) => void;
}) {
  const { t } = useTranslation();
  const startupPending =
    status?.state === RuntimeState.Starting || status?.state === "restarting";
  const { canInterrupt, blocked, running, install } = lifecycleControlState(
    status,
    disabled,
  );
  return (
    <>
      <Button
        iconOnly
        variant="success"
        title={t("common.start")}
        aria-label={t("common.start")}
        disabled={blocked || install || running}
        aria-busy={startupPending}
        onClick={() => onAction(ServerAction.Start)}
      >
        <NavigationIcon name="play" filled />
      </Button>
      <Button
        iconOnly
        variant="warning"
        title={t("common.stop")}
        aria-label={t("common.stop")}
        disabled={!canInterrupt && (blocked || !running)}
        onClick={() => onAction(ServerAction.Stop)}
      >
        <NavigationIcon name="stop" filled />
      </Button>
      <Button
        iconOnly
        variant="secondary"
        title={t("common.restart")}
        aria-label={t("common.restart")}
        disabled={blocked || !running}
        onClick={() => onAction(ServerAction.Restart)}
      >
        <NavigationIcon name="restart" />
      </Button>
      <Button
        iconOnly
        variant="danger"
        title={t("common.kill")}
        aria-label={t("common.kill")}
        disabled={!canInterrupt && (blocked || !running)}
        onClick={() => onAction(ServerAction.Kill)}
      >
        <NavigationIcon name="kill" />
      </Button>
      {(install || canConfigure) && (
        <Button
          variant={install ? "primary" : "warning"}
          disabled={blocked || running}
          onClick={() =>
            onAction(install ? ServerAction.Install : ServerAction.Reinstall)
          }
        >
          {install ? t("common.install") : t("common.reinstall")}
        </Button>
      )}
    </>
  );
}
