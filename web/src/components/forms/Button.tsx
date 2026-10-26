import type { ComponentProps, ReactNode } from "react";
import "./button.css";

type ButtonVariant =
  "default" | "primary" | "secondary" | "success" | "warning" | "danger";

export function Button({
  variant = "default",
  loading = false,
  loadingLabel,
  iconOnly = false,
  size = "default",
  disabled,
  "aria-busy": ariaBusy,
  className,
  children,
  ...props
}: ComponentProps<"button"> & {
  variant?: ButtonVariant;
  loading?: boolean;
  loadingLabel?: ReactNode;
  iconOnly?: boolean;
  size?: "default" | "small";
}) {
  return (
    <button
      {...props}
      className={[
        "button",
        variant !== "default" && `button-${variant}`,
        iconOnly && "button-icon",
        size === "small" && "button-small",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
      disabled={loading ? true : disabled}
      aria-busy={loading ? true : ariaBusy}
    >
      {loading ? (loadingLabel ?? children) : children}
    </button>
  );
}

export function ButtonLink({
  variant = "default",
  iconOnly = false,
  className,
  ...props
}: ComponentProps<"a"> & { variant?: ButtonVariant; iconOnly?: boolean }) {
  return (
    <a
      {...props}
      className={[
        "button",
        variant !== "default" && `button-${variant}`,
        iconOnly && "button-icon",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    />
  );
}
