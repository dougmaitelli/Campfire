import type { EmberVariable } from "../../api/embers";
import { FormField } from "../forms/FormField";
import { TextInput } from "../forms/TextInput";
import { ToggleSwitch } from "../forms/ToggleSwitch";

export function ServerVariableField({
  variable,
  value,
  disabled,
  onChange,
}: {
  variable: EmberVariable;
  value: string;
  disabled: boolean;
  onChange: (value: string) => void;
}) {
  return (
    <FormField label={variable.name}>
      {variable.validation?.type === "boolean" ? (
        <ToggleSwitch
          id={`variable-${variable.name}`}
          aria-label={variable.name}
          disabled={disabled || !variable.editable}
          checked={value === "1"}
          onChange={(event) => onChange(event.target.checked ? "1" : "0")}
        />
      ) : (
        <TextInput
          id={`variable-${variable.name}`}
          disabled={disabled}
          required={variable.required}
          readOnly={!variable.editable}
          maxLength={8192}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </FormField>
  );
}
