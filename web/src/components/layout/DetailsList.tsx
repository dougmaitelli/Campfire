import type { ComponentProps, ReactNode } from "react";
import { ChangeIndicator } from "../status/ChangeIndicator";
import type { ConfigurationChange } from "../../utils/serverConfigurationChanges";
import "./details-list.css";

export function DetailsList({ className, ...props }: ComponentProps<"dl">) {
  return (
    <dl
      {...props}
      className={["details-list", className].filter(Boolean).join(" ")}
    />
  );
}

export function DetailItem({
  label,
  children,
  valueClassName,
  change,
  ...props
}: Omit<ComponentProps<"div">, "children"> & {
  label: ReactNode;
  children: ReactNode;
  valueClassName?: string;
  change?: ConfigurationChange | undefined;
}) {
  return (
    <div {...props} data-change={change}>
      <dt>
        {label}
        {change && <ChangeIndicator change={change} />}
      </dt>
      <dd className={valueClassName}>{children}</dd>
    </div>
  );
}
