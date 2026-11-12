import { useTranslation } from "react-i18next";
import type { IntegrationConnection } from "../../api/integrations";
import { FormField } from "../forms/FormField";
import { Select } from "../forms/Select";

export function IntegrationConnectionField({
  connections,
  value,
  onChange,
  disabled,
  readOnly,
}: {
  connections: IntegrationConnection[];
  value: string;
  onChange: (value: string) => void;
  disabled: boolean;
  readOnly: boolean;
}) {
  const { t } = useTranslation();
  const label = (connection: IntegrationConnection) =>
    `${t(`integrations.${connection.provider}`, { defaultValue: connection.provider })} · ${connection.name}`;
  return (
    <FormField label={t("serverIntegrations.connection")}>
      <Select
        required
        value={value}
        disabled={disabled || readOnly}
        onValueChange={onChange}
      >
        <option value="">{t("serverIntegrations.chooseConnection")}</option>
        {connections.map((connection) => (
          <option
            key={connection.id}
            value={connection.id}
            disabled={!connection.enabled}
          >
            {label(connection)}
          </option>
        ))}
      </Select>
    </FormField>
  );
}
