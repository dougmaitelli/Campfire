import type { ComponentProps } from "react";
import "./list-toolbar.css";

export function ListToolbar({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      {...props}
      className={["list-toolbar", className].filter(Boolean).join(" ")}
    />
  );
}
