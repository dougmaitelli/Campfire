import { useTranslation } from "react-i18next";
import type { ComponentProps } from "react";
import type { AccessUser } from "../../api/access";
import { formatUserLabel } from "../../utils/formatUserLabel";
import { Select } from "./Select";

type Props = Omit<
  ComponentProps<typeof Select>,
  "children" | "value" | "defaultValue" | "onValueChange"
> & {
  users: (AccessUser & { disabled?: boolean })[];
  value: number;
  onValueChange: (id: number) => void;
};

export function UserSelect({ users, value, onValueChange, ...props }: Props) {
  const { t } = useTranslation();
  return (
    <Select
      {...props}
      value={value}
      onValueChange={(selected) => onValueChange(Number(selected))}
    >
      <option value={0} disabled>
        {users.length
          ? t("userSelect.selectAUser")
          : t("userSelect.noUsersAvailable")}
      </option>
      {users.map((user) => (
        <option key={user.id} value={user.id}>
          {formatUserLabel(user)}
          {user.disabled ? " (disabled)" : ""}
        </option>
      ))}
    </Select>
  );
}
