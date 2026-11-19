import { RuntimeState, ServerAction } from "../../contracts/domains";
import { useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { serverAction, type RuntimeStatus } from "../../api/runtime";
import { Button } from "../forms/Button";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { ActionGroup } from "../layout/ActionGroup";
import { lifecycleControlState } from "./lifecycleControlState";

type QuickAction =
  ServerAction.Start | ServerAction.Stop | ServerAction.Restart;

export function ServerQuickControls({
  id,
  name,
  status,
  disabled,
  canOperate,
  onStatus,
  onError,
  children,
}: {
  id: string;
  name: string;
  status: RuntimeStatus | undefined;
  disabled: boolean;
  canOperate: boolean;
  onStatus: (status: RuntimeStatus) => void;
  onError: (message: string | null) => void;
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  const [action, setAction] = useState<QuickAction | null>(null);
  const pending = useRef(false);
  const startupPending =
    action === ServerAction.Start ||
    status?.state === RuntimeState.Starting ||
    status?.state === RuntimeState.Restarting;
  const { blocked, running, install, canInterrupt } = lifecycleControlState(
    status,
    disabled || !canOperate || action !== null,
  );
  async function act(next: QuickAction) {
    if (pending.current) return;
    pending.current = true;
    setAction(next);
    onError(null);
    try {
      onStatus(await serverAction(id, next));
    } catch (cause) {
      onError(
        t("serverQuickControls.failed", {
          name,
          message:
            cause instanceof Error
              ? cause.message
              : t("serverOperationsPage.unableToRunServerAction"),
        }),
      );
    } finally {
      pending.current = false;
      setAction(null);
    }
  }
  return (
    <ActionGroup>
      {canOperate && (
        <>
          <Button
            iconOnly
            variant="success"
            disabled={blocked || install || running}
            aria-label={t("serverQuickControls.start", { name })}
            title={t("common.start")}
            loading={action === ServerAction.Start}
            aria-busy={startupPending}
            onClick={() => void act(ServerAction.Start)}
          >
            <NavigationIcon
              name={action === ServerAction.Start ? "loading" : "play"}
              filled={action !== ServerAction.Start}
            />
          </Button>
          <Button
            iconOnly
            variant="warning"
            disabled={!canInterrupt && (blocked || !running)}
            aria-label={t("serverQuickControls.stop", { name })}
            title={t("common.stop")}
            loading={action === ServerAction.Stop}
            onClick={() => void act(ServerAction.Stop)}
          >
            <NavigationIcon
              name={action === ServerAction.Stop ? "loading" : "stop"}
              filled={action !== ServerAction.Stop}
            />
          </Button>
          <Button
            iconOnly
            variant="secondary"
            disabled={blocked || !running}
            aria-label={t("serverQuickControls.restart", { name })}
            title={t("common.restart")}
            loading={action === ServerAction.Restart}
            onClick={() => void act(ServerAction.Restart)}
          >
            <NavigationIcon
              name={action === ServerAction.Restart ? "loading" : "restart"}
            />
          </Button>
        </>
      )}
      {children}
    </ActionGroup>
  );
}
