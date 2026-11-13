import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "../forms/Button";
import { ActionMenu, type ActionMenuItem } from "../forms/ActionMenu";
import { NavigationIcon } from "../navigation/NavigationIcon";
import "./page-header.css";

interface PageHeaderPrimaryAction {
  label: string;
  compactLabel: string;
  disabled?: boolean;
  onClick: () => void;
}

interface PageHeaderProps {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  primaryAction?: PageHeaderPrimaryAction | undefined;
  secondaryActions?: ActionMenuItem[];
  leading?: ReactNode;
  badge?: ReactNode;
}

export function PageHeader({
  title,
  description,
  actions,
  primaryAction,
  secondaryActions = [],
  leading,
  badge,
}: PageHeaderProps) {
  const { t } = useTranslation();
  return (
    <header className="page-header">
      <div className="page-header-content">
        {leading && <div className="page-header-leading">{leading}</div>}
        <div className="page-header-title">
          <h1 id="page-title">{title}</h1>
          {badge}
        </div>
        {description && <p className="page-description">{description}</p>}
      </div>
      {(!!primaryAction || secondaryActions.length > 0 || !!actions) && (
        <div className="page-actions">
          {primaryAction && (
            <Button
              variant="primary"
              className="page-header-primary-action"
              aria-label={primaryAction.label}
              title={primaryAction.label}
              disabled={primaryAction.disabled}
              onClick={primaryAction.onClick}
            >
              <span className="page-header-primary-icon" aria-hidden="true">
                <NavigationIcon name="plus" />
              </span>
              <span className="page-header-action-label" aria-hidden="true">
                {primaryAction.label}
              </span>
              <span
                className="page-header-action-compact-label"
                aria-hidden="true"
              >
                {primaryAction.compactLabel}
              </span>
            </Button>
          )}
          {secondaryActions.length > 0 && (
            <>
              <div className="page-header-secondary-actions">
                {secondaryActions.map((action) => (
                  <Button
                    key={action.label}
                    variant={action.tone ?? "default"}
                    disabled={action.disabled}
                    loading={action.loading === true}
                    loadingLabel={action.loadingLabel}
                    onClick={action.onSelect}
                  >
                    {action.label}
                  </Button>
                ))}
              </div>
              <div className="page-header-secondary-menu">
                <ActionMenu
                  label={t("common.actions")}
                  items={secondaryActions}
                />
              </div>
            </>
          )}
          {actions}
        </div>
      )}
    </header>
  );
}
