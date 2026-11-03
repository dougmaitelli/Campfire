import { useState, type ComponentProps } from "react";
import { useTranslation } from "react-i18next";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { TextInput } from "./TextInput";
import "./date-time-input.css";

export function DateTimeInput(props: Omit<ComponentProps<"input">, "type">) {
  const { t } = useTranslation();
  const [value, setValue] = useState(props.defaultValue ?? "");
  return (
    <div className="date-time-input" data-empty={!(props.value ?? value)}>
      <TextInput
        {...props}
        type="datetime-local"
        onChange={(event) => {
          setValue(event.currentTarget.value);
          props.onChange?.(event);
        }}
      />
      <span className="date-time-input-placeholder" aria-hidden="true">
        {props.placeholder ?? t("dateTimeInput.formatHint")}
      </span>
      <span className="date-time-input-icon" aria-hidden="true">
        <NavigationIcon name="calendar" />
      </span>
    </div>
  );
}
