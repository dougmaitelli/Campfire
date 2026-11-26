import { useState, type ReactNode } from "react";
import { Check, Copy } from "lucide-react";
import "./copyable-text.css";

export function CopyableText({
  value,
  children,
  copyLabel,
  copiedLabel,
  failedLabel,
  disabled = false,
  className,
}: {
  value: string;
  children?: ReactNode;
  copyLabel: string;
  copiedLabel: string;
  failedLabel: string;
  disabled?: boolean;
  className?: string;
}) {
  const [copiedValue, setCopiedValue] = useState<string | null>(null);
  const [failedValue, setFailedValue] = useState<string | null>(null);
  const copied = copiedValue === value;
  const failed = failedValue === value;
  const label = copied ? copiedLabel : copyLabel;
  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopiedValue(value);
      setFailedValue(null);
    } catch {
      setCopiedValue(null);
      setFailedValue(value);
    }
  }
  return (
    <span className={["copyable-text", className].filter(Boolean).join(" ")}>
      <span className="copyable-text-value">{children ?? value}</span>
      <button
        type="button"
        className="copyable-text-button"
        aria-label={label}
        title={label}
        disabled={disabled}
        onClick={() => void copy()}
      >
        {copied ? (
          <Check size="1em" aria-hidden="true" />
        ) : (
          <Copy size="1em" aria-hidden="true" />
        )}
      </button>
      <span
        className={failed ? "copyable-text-error" : "visually-hidden"}
        role="status"
      >
        {failed ? failedLabel : copied ? copiedLabel : ""}
      </span>
    </span>
  );
}
