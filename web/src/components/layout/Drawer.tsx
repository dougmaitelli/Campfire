import { useTranslation } from "react-i18next";
import { Button } from "../forms/Button";
import { useEffect, useId, useRef } from "react";
import type { ReactNode } from "react";
import "./drawer.css";
import { useNavigationGuard } from "../../hooks/useNavigationGuard";

interface DrawerProps {
  title: ReactNode;
  description?: string;
  busy: boolean;
  onClose: () => void;
  children: ReactNode;
  footerLeft: ReactNode;
  footerRight: ReactNode;
  actions?: ReactNode;
  leading?: ReactNode;
  embedded?: boolean;
  unsavedChanges?: boolean;
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
  embedded = false,
  unsavedChanges = false,
}: DrawerProps) {
  const { t } = useTranslation();
  const ref = useRef<HTMLDialogElement>(null);
  const id = useId();
  useNavigationGuard(embedded, busy, unsavedChanges);
  useEffect(() => {
    if (embedded) return;
    const dialog = ref.current;
    dialog?.showModal();
    dialog?.querySelector<HTMLInputElement>("input, textarea")?.focus();
    return () => dialog?.close();
  }, [embedded]);
  const actionBar = actions != null && (
    <div
      className="drawer-actions"
      role="group"
      aria-label={t("common.actions")}
    >
      {actions}
    </div>
  );
  const footer = (footerLeft != null || footerRight != null) && (
    <footer className="drawer-footer">
      <div className="drawer-footer-left">{footerLeft}</div>
      <div className="drawer-footer-right">{footerRight}</div>
    </footer>
  );
  if (embedded)
    return (
      <section className="drawer-form drawer-panel" aria-labelledby={id}>
        {leading && <div className="drawer-leading">{leading}</div>}
        <header className="drawer-heading">
          <div>
            <h2 id={id} className="drawer-title">
              {title}
            </h2>
            {description && <p>{description}</p>}
          </div>
          {actionBar}
        </header>
        <div className="drawer-content-area">{children}</div>
        {footer}
      </section>
    );
  return (
    <dialog
      ref={ref}
      className="drawer"
      aria-labelledby={id}
      aria-describedby={description ? `${id}-description` : undefined}
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) onClose();
      }}
    >
      <div className="drawer-form">
        {leading && <div className="drawer-leading">{leading}</div>}
        <header className="drawer-heading">
          <div>
            <h2 id={id} className="drawer-title">
              {title}
            </h2>
            {description && <p id={`${id}-description`}>{description}</p>}
          </div>
          <Button
            type="button"
            iconOnly
            aria-label={t("common.close")}
            disabled={busy}
            onClick={onClose}
          >
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.8"
              strokeLinecap="round"
              aria-hidden="true"
            >
              <path d="M6 6l12 12M18 6 6 18" />
            </svg>
          </Button>
        </header>
        {actionBar}
        <div className="drawer-content-area">{children}</div>
        {footer}
      </div>
    </dialog>
  );
}
