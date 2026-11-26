import { useTranslation } from "react-i18next";
import { X } from "lucide-react";
import { Button } from "../forms/Button";
import { ActionMenu, type ActionMenuItem } from "../forms/ActionMenu";
import { useId, type ReactNode } from "react";
import { BaseDialog, type BaseDialogProps } from "./BaseDialog";
import { Panel } from "./Panel";
import "./drawer.css";

interface DrawerProps extends Pick<BaseDialogProps, "busy" | "onClose"> {
  title: ReactNode;
  description?: string;
  children: ReactNode;
  footerLeft: ReactNode;
  footerRight: ReactNode;
  actions?: ActionMenuItem[] | undefined;
  headingActions?: ReactNode;
  leading?: ReactNode;
}

export function Drawer({
  title,
  description,
  busy,
  onClose,
  children,
  footerLeft,
  footerRight,
  actions,
  headingActions,
  leading,
}: DrawerProps) {
  const { t } = useTranslation();
  const id = useId();
  return (
    <BaseDialog
      busy={busy}
      onClose={onClose}
      className="drawer"
      aria-labelledby={id}
      aria-describedby={description ? id + "-description" : undefined}
    >
      <Panel
        title={title}
        description={description}
        headingID={id}
        leading={leading}
        headingActions={
          <>
            {headingActions}
            {actions && actions.length > 0 && (
              <ActionMenu label={t("common.actions")} items={actions} />
            )}
            <Button
              type="button"
              iconOnly
              aria-label={t("common.close")}
              disabled={busy}
              onClick={onClose}
            >
              <X size={18} aria-hidden="true" />
            </Button>
          </>
        }
        footerLeft={footerLeft}
        footerRight={footerRight}
      >
        {children}
      </Panel>
    </BaseDialog>
  );
}
