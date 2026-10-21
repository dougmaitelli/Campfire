import type { ComponentProps } from "react";
import { NavigationIcon } from "./NavigationIcon";
import "./arrow-link.css";

export function ArrowLink({
  children,
  className,
  ...props
}: ComponentProps<"a">) {
  return (
    <a
      {...props}
      className={["arrow-link", className].filter(Boolean).join(" ")}
    >
      {children}
      <NavigationIcon name="arrow-right" />
    </a>
  );
}
