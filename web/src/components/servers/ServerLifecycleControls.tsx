import { RuntimeState, type LifecycleAction } from "../../contracts/domains";
import { useTranslation } from "react-i18next";
import { Button } from "../forms/Button";
import { ActionMenu, type ActionMenuItem } from "../forms/ActionMenu";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { type RuntimeStatus, ServerAction } from "../../api/runtime";
import { lifecycleControlState } from "./lifecycleControlState";

export function ServerLifecycleControls({
  status,
  disabled,
  canReinstall,
  canOperate = true,
  actions = [],
  onAction,
}: {
  status: RuntimeStatus | undefined;
  disabled: boolean;
  canReinstall: boolean;
  canOperate?: boolean;
  actions?: ActionMenuItem[];
  onAction: (action: LifecycleAction) => void;
}) {
  const { t } = useTranslation();
  const startupPending =
    status?.state === RuntimeState.Starting || status?.state === "restarting";
  const { canInterrupt, blocked, running, install } = lifecycleControlState(
    status,
    disabled,
  );
  const items: ActionMenuItem[] = actions.filter(
    (item) => item.tone !== "danger",
  );
  if (canOperate)
    items.push({
      label: t("common.kill"),
      icon: "kill",
      tone: "danger",
      disabled: !canInterrupt && (blocked || !running),
      onSelect: () => onAction(ServerAction.Kill),
    });
  if (canOperate && canReinstall) {
    items.push({
      label: t("common.reinstall"),
      icon: "download",
      tone: "warning",
      disabled: blocked || install || running,
      onSelect: () => onAction(ServerAction.Reinstall),
    });
  }
  items.push(...actions.filter((item) => item.tone === "danger"));
  return (
    <>
      {canOperate && (
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
        </>
      )}
      {items.length > 0 && (
        <ActionMenu label={t("common.moreServerActions")} items={items} />
      )}
    </>
  );
}
