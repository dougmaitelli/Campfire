import { useTranslation } from "react-i18next";
import type { ComponentProps } from "react";
import type { ConfigurationChange } from "../../utils/serverConfigurationChanges";
import "./change-indicator.css";

export function ChangeIndicator({
  change,
  className,
  ...props
}: Omit<ComponentProps<"span">, "children"> & { change: ConfigurationChange }) {
  const { t } = useTranslation();
  return (
    <span
      {...props}
      className={["change-indicator", className].filter(Boolean).join(" ")}
    >
      {t(`changes.${change}`)}
    </span>
  );
}
