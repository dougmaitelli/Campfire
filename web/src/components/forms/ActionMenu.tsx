import { useId, useRef, type ComponentProps } from "react";
import { ChevronDown } from "lucide-react";
import { Button } from "./Button";
import { NavigationIcon } from "../navigation/NavigationIcon";
import "./action-menu.css";

export interface ActionMenuItem {
  label: string;
  icon: ComponentProps<typeof NavigationIcon>["name"];
  disabled?: boolean;
  loading?: boolean;
  loadingLabel?: string;
  tone?: "danger" | "warning";
  onSelect: () => void;
}

export function ActionMenu({
  label,
  items,
  iconOnly = true,
  variant = "default",
  disabled = false,
}: {
  label: string;
  items: ActionMenuItem[];
  iconOnly?: boolean;
  variant?: ComponentProps<typeof Button>["variant"];
  disabled?: boolean;
}) {
  const id = useId();
  const menu = useRef<HTMLDivElement>(null);
  return (
    <>
      <Button
        type="button"
        iconOnly={iconOnly}
        variant={variant}
        disabled={disabled}
        className={iconOnly ? undefined : "action-menu-trigger"}
        title={label}
        aria-label={label}
        aria-haspopup="menu"
        popoverTarget={id}
      >
        {iconOnly ? (
          <NavigationIcon name="more" />
        ) : (
          <>
            {label}
            <ChevronDown size={16} aria-hidden="true" />
          </>
        )}
      </Button>
      <div
        id={id}
        ref={menu}
        className="action-menu"
        popover="auto"
        role="menu"
        aria-label={label}
        tabIndex={-1}
        onToggle={(event) => {
          if (event.newState === "open") {
            (
              event.currentTarget.querySelector<HTMLButtonElement>(
                "button:not(:disabled)",
              ) ?? event.currentTarget
            ).focus({ preventScroll: true });
          }
        }}
        onBlur={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget))
            event.currentTarget.hidePopover();
        }}
        onKeyDown={(event) => {
          const buttons = Array.from(
            event.currentTarget.querySelectorAll<HTMLButtonElement>(
              "button:not(:disabled)",
            ),
          );
          if (buttons.length === 0) return;
          const current = buttons.findIndex(
            (button) => button === document.activeElement,
          );
          let next: number;
          switch (event.key) {
            case "ArrowDown":
              next = (current + 1) % buttons.length;
              break;
            case "ArrowUp":
              next = (current - 1 + buttons.length) % buttons.length;
              break;
            case "Home":
              next = 0;
              break;
            case "End":
              next = buttons.length - 1;
              break;
            default:
              return;
          }
          event.preventDefault();
          buttons[next]?.focus({ preventScroll: true });
        }}
      >
        {items.map((item) => (
          <button
            key={item.label}
            type="button"
            role="menuitem"
            tabIndex={-1}
            className={item.tone ? `action-menu-${item.tone}` : undefined}
            disabled={
              disabled || item.disabled === true || item.loading === true
            }
            aria-busy={item.loading}
            onClick={() => {
              menu.current?.hidePopover();
              item.onSelect();
            }}
          >
            <NavigationIcon name={item.icon} />
            {item.loading ? (item.loadingLabel ?? item.label) : item.label}
          </button>
        ))}
      </div>
    </>
  );
}
