import { useTranslation } from "react-i18next";
import { X } from "lucide-react";
import { Button } from "../forms/Button";
import { ActionMenu, type ActionMenuItem } from "../forms/ActionMenu";
import { useEffect, useId, useRef, type ReactNode } from "react";
import { Panel } from "./Panel";
import "./drawer.css";

interface DrawerProps {
  title: ReactNode;
  description?: string;
  busy: boolean;
  onClose: () => void;
  children: ReactNode;
  footerLeft: ReactNode;
  footerRight: ReactNode;
  actions?: ActionMenuItem[] | undefined;
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
  leading,
}: DrawerProps) {
  const { t } = useTranslation();
  const ref = useRef<HTMLDialogElement>(null);
  const id = useId();
  useEffect(() => {
    const dialog = ref.current;
    dialog?.showModal();
    dialog?.querySelector<HTMLInputElement>("input, textarea")?.focus();
    return () => dialog?.close();
  }, []);
  return (
    <dialog
      ref={ref}
      className="drawer"
      aria-labelledby={id}
      aria-describedby={description ? id + "-description" : undefined}
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) onClose();
      }}
    >
      <Panel
        title={title}
        description={description}
        headingID={id}
        leading={leading}
        headingActions={
          <>
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
    </dialog>
  );
}
