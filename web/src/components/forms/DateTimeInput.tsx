import type { ComponentProps } from "react";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { TextInput } from "./TextInput";
import "./date-time-input.css";

export function DateTimeInput(props: Omit<ComponentProps<"input">, "type">) {
  return (
    <div className="date-time-input">
      <TextInput {...props} type="datetime-local" />
      <span className="date-time-input-icon" aria-hidden="true">
        <NavigationIcon name="calendar" />
      </span>
    </div>
  );
}
