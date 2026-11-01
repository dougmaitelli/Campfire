import { useTranslation } from "react-i18next";
import { useEffect, useId, useRef, useState } from "react";
import type { NavigationItem } from "../../routing/navigation";
import { Sidebar } from "./Sidebar";
import { NavigationIcon } from "./NavigationIcon";
import { Button } from "../forms/Button";
import "./mobile-navigation.css";

export function MobileNavigation({ page }: { page: NavigationItem }) {
  const { t } = useTranslation();
  const [navigationOpen, setNavigationOpen] = useState(false);
  const navigationDialog = useRef<HTMLDialogElement>(null);
  const navigationID = useId();
  const closeNavigation = () => navigationDialog.current?.close();
  useEffect(() => {
    if (!navigationOpen) return;
    const dialog = navigationDialog.current;
    dialog?.showModal();
    dialog?.querySelector<HTMLAnchorElement>('[aria-current="page"]')?.focus();
    return () => dialog?.close();
  }, [navigationOpen]);
  useEffect(() => {
    const desktop = window.matchMedia("(width > 900px)");
    const closeOnDesktop = () => {
      if (desktop.matches) navigationDialog.current?.close();
    };
    desktop.addEventListener("change", closeOnDesktop);
    return () => desktop.removeEventListener("change", closeOnDesktop);
  }, []);
  return (
    <>
      <Button
        className="navigation-toggle"
        iconOnly
        aria-label={t("dashboardLayout.openNavigation")}
        aria-expanded={navigationOpen}
        aria-haspopup="dialog"
        aria-controls={navigationID}
        onClick={() => setNavigationOpen(true)}
      >
        <NavigationIcon name="menu" />
      </Button>
      <dialog
        ref={navigationDialog}
        id={navigationID}
        className="mobile-navigation"
        aria-label={t("sidebar.mainNavigation")}
        onClose={(event) => {
          if (event.target === event.currentTarget) setNavigationOpen(false);
        }}
        onClick={(event) => {
          if (event.target === event.currentTarget) event.currentTarget.close();
        }}
      >
        <Sidebar
          page={page}
          onNavigate={closeNavigation}
          onClose={closeNavigation}
        />
      </dialog>
    </>
  );
}
