import type { ComponentProps, ReactNode } from "react";
import "./button.css";

type ButtonVariant = "default" | "primary" | "warning" | "danger";

export function Button({
  variant = "default",
  loading = false,
  loadingLabel,
  disabled,
  "aria-busy": ariaBusy,
  className,
  children,
  ...props
}: ComponentProps<"button"> & {
  variant?: ButtonVariant;
  loading?: boolean;
  loadingLabel?: ReactNode;
}) {
  return (
    <button
      {...props}
      className={[
        "button",
        variant !== "default" && `button-${variant}`,
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
  className,
  ...props
}: ComponentProps<"a"> & { variant?: ButtonVariant }) {
  return (
    <a
      {...props}
      className={[
        "button",
        variant !== "default" && `button-${variant}`,
        className,
      ]
        .filter(Boolean)
        .join(" ")}
    />
  );
}
