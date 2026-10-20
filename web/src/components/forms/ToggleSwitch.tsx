import type { ComponentProps } from "react";
import "./toggle-switch.css";

export function ToggleSwitch(
  props: Omit<ComponentProps<"input">, "type" | "role" | "aria-label"> & {
    "aria-label": string;
  },
) {
  return (
    <input
      {...props}
      className={["toggle-switch", props.className].filter(Boolean).join(" ")}
      type="checkbox"
      role="switch"
    />
  );
}
