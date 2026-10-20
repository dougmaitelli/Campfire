import { useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { serverAction, type RuntimeStatus } from "../../api/runtime";
import { Button } from "../forms/Button";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { ActionGroup } from "../layout/ActionGroup";
import { lifecycleControlState } from "./lifecycleControlState";

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
  const [action, setAction] = useState<"start" | "stop" | null>(null);
  const pending = useRef(false);
  const { blocked, running, install, canInterrupt } = lifecycleControlState(
    status,
    disabled || !canOperate || action !== null,
  );
  async function act(next: "start" | "stop") {
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
      <Button
        iconOnly
        variant="success"
        disabled={blocked || install || running}
        aria-label={t("serverQuickControls.start", { name })}
        title={t("common.start")}
        loading={action === "start"}
        onClick={() => void act("start")}
      >
        <NavigationIcon
          name={action === "start" ? "loading" : "play"}
          filled={action !== "start"}
        />
      </Button>
      <Button
        iconOnly
        variant="primary"
        disabled={!canInterrupt && (blocked || !running)}
        aria-label={t("serverQuickControls.stop", { name })}
        title={t("common.stop")}
        loading={action === "stop"}
        onClick={() => void act("stop")}
      >
        <NavigationIcon
          name={action === "stop" ? "loading" : "stop"}
          filled={action !== "stop"}
        />
      </Button>
      {children}
    </ActionGroup>
  );
}
