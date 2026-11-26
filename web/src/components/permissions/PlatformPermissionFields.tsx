import { useTranslation } from "react-i18next";
import type { PermissionID } from "../../api/permissions";
import { CheckboxField } from "../forms/CheckboxField";
import "./permission-fields.css";

// Callers supply the allowed choices: custom platform roles remain restricted
// to server creation, while token scopes can restrict any platform capability.
export function PlatformPermissionFields({
  choices,
  selected,
  onChange,
  disabled = false,
}: {
  choices: PermissionID[];
  selected: PermissionID[];
  onChange: (selected: PermissionID[]) => void;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      {choices.map((id) => (
        <CheckboxField
          key={id}
          checked={selected.includes(id)}
          disabled={disabled}
          onChange={(event) =>
            onChange(
              event.target.checked
                ? [...new Set([...selected, id])]
                : selected.filter((permission) => permission !== id),
            )
          }
        >
          {t(`permissionNames.${id.replaceAll(".", "_")}`, {
            defaultValue: id,
          })}
        </CheckboxField>
      ))}
    </>
  );
}
