import type { ComponentProps, ReactNode } from "react";
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
  ...props
}: Omit<ComponentProps<"div">, "children"> & {
  label: ReactNode;
  children: ReactNode;
  valueClassName?: string;
}) {
  return (
    <div {...props}>
      <dt>{label}</dt>
      <dd className={valueClassName}>{children}</dd>
    </div>
  );
}
