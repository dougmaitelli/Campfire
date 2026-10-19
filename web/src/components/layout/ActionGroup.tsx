import type { ComponentProps } from "react";
import "./action-group.css";

export function ActionGroup({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      {...props}
      className={["action-group", className].filter(Boolean).join(" ")}
    />
  );
}
