import { useTranslation } from "react-i18next";
import { Button } from "../forms/Button";
import { NavigationIcon } from "../navigation/NavigationIcon";
import type { RuntimeStatus, ServerAction } from "../../api/runtime";
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
  onAction: (action: ServerAction) => void;
}) {
  const { t } = useTranslation();
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
        onClick={() => onAction("start")}
      >
        <NavigationIcon name="play" filled />
      </Button>
      <Button
        iconOnly
        variant="warning"
        title={t("common.stop")}
        aria-label={t("common.stop")}
        disabled={!canInterrupt && (blocked || !running)}
        onClick={() => onAction("stop")}
      >
        <NavigationIcon name="stop" filled />
      </Button>
      <Button
        iconOnly
        variant="secondary"
        title={t("common.restart")}
        aria-label={t("common.restart")}
        disabled={blocked || !running}
        onClick={() => onAction("restart")}
      >
        <NavigationIcon name="restart" />
      </Button>
      <Button
        iconOnly
        variant="danger"
        title={t("common.kill")}
        aria-label={t("common.kill")}
        disabled={!canInterrupt && (blocked || !running)}
        onClick={() => onAction("kill")}
      >
        <NavigationIcon name="kill" />
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
