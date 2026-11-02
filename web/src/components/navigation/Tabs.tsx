import type { KeyboardEvent } from "react";
import "./tabs.css";

export function Tabs<T extends string>({
  label,
  items,
  value,
  onChange,
}: {
  label: string;
  items: readonly {
    value: T;
    label: string;
    id: string;
    panelID: string;
    disabled?: boolean;
  }[];
  value: T;
  onChange: (value: T) => void;
}) {
  function navigate(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    const enabled = items.filter((item) => !item.disabled);
    const current = enabled.findIndex(
      (item) => item.value === items[index]?.value,
    );
    const next =
      event.key === "Home"
        ? 0
        : event.key === "End"
          ? enabled.length - 1
          : event.key === "ArrowRight"
            ? (current + 1) % enabled.length
            : event.key === "ArrowLeft"
              ? (current + enabled.length - 1) % enabled.length
              : null;
    if (next === null || !enabled[next]) return;
    event.preventDefault();
    const item = enabled[next];
    onChange(item.value);
    document.getElementById(item.id)?.focus();
  }
  return (
    <div className="tab-strip" role="tablist" aria-label={label}>
      {items.map((item, index) => (
        <button
          key={item.value}
          id={item.id}
          type="button"
          role="tab"
          aria-selected={value === item.value}
          aria-controls={item.panelID}
          tabIndex={value === item.value ? 0 : -1}
          disabled={item.disabled}
          onClick={() => onChange(item.value)}
          onKeyDown={(event) => navigate(event, index)}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}
