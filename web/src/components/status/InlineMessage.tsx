import type { ComponentProps } from "react";
import "./inline-message.css";

export function InlineMessage({
  tone = "error",
  className,
  role,
  ...props
}: ComponentProps<"p"> & { tone?: "error" | "warning" | "info" | "success" }) {
  return (
    <p
      {...props}
      role={role ?? (tone === "error" ? "alert" : "status")}
      className={[
        "inline-message",
        tone === "error" ? "error-message" : `inline-message-${tone}`,
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    />
  );
}
