import { useEffect, useId, useRef } from "react";
import type { ReactNode } from "react";
import "./drawer.css";
import { useNavigationGuard } from "../../hooks/useNavigationGuard";

interface DrawerProps {
  title: string;
  description: string;
  busy: boolean;
  onClose: () => void;
  children: ReactNode;
  footerLeft: ReactNode;
  footerRight: ReactNode;
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
  embedded = false,
  unsavedChanges = false,
}: DrawerProps) {
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
  if (embedded)
    return (
      <section className="drawer-form operations-panel" aria-labelledby={id}>
        <header className="drawer-heading">
          <div>
            <h2 id={id}>{title}</h2>
            <p>{description}</p>
          </div>
        </header>
        <div className="drawer-content-area">{children}</div>
        {(footerLeft != null || footerRight != null) && (
          <footer className="drawer-footer">
            <div className="drawer-footer-left">{footerLeft}</div>
            <div className="drawer-footer-right">{footerRight}</div>
          </footer>
        )}
      </section>
    );
  return (
    <dialog
      ref={ref}
      className="drawer"
      aria-labelledby={id}
      aria-describedby={`${id}-description`}
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) onClose();
      }}
    >
      <div className="drawer-form">
        <header className="drawer-heading">
          <div>
            <h2 id={id}>{title}</h2>
            <p id={`${id}-description`}>{description}</p>
          </div>
          <button
            type="button"
            className="button button-icon"
            aria-label="Close"
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
          </button>
        </header>
        <div className="drawer-content-area">{children}</div>
        <footer className="drawer-footer">
          <div className="drawer-footer-left">{footerLeft}</div>
          <div className="drawer-footer-right">{footerRight}</div>
        </footer>
      </div>
    </dialog>
  );
}
