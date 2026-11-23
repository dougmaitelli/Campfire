import { useTranslation } from "react-i18next";
import { useEffect, useId, useState } from "react";
import { BaseDialog } from "../layout/BaseDialog";
import type { NavigationItem } from "../../routing/navigation";
import { Sidebar } from "./Sidebar";
import { NavigationIcon } from "./NavigationIcon";
import { Button } from "../forms/Button";
import "./mobile-navigation.css";

export function MobileNavigation({ page }: { page: NavigationItem }) {
  const { t } = useTranslation();
  const [navigationOpen, setNavigationOpen] = useState(false);
  const navigationID = useId();
  const closeNavigation = () => setNavigationOpen(false);
  useEffect(() => {
    const desktop = window.matchMedia("(width > 900px)");
    const closeOnDesktop = () => {
      if (desktop.matches) setNavigationOpen(false);
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
      <BaseDialog
        open={navigationOpen}
        busy={false}
        onClose={closeNavigation}
        dismissOnBackdrop
        initialFocus='[aria-current="page"]'
        id={navigationID}
        className="mobile-navigation"
        aria-label={t("sidebar.mainNavigation")}
      >
        <Sidebar
          page={page}
          onNavigate={closeNavigation}
          onClose={closeNavigation}
        />
      </BaseDialog>
    </>
  );
}
