import type { ComponentProps } from "react";
import "./selection-control.css";

export function Checkbox(props: Omit<ComponentProps<"input">, "type">) {
  return (
    <input
      {...props}
      className={["selection-control", props.className]
        .filter(Boolean)
        .join(" ")}
      type="checkbox"
    />
  );
}
