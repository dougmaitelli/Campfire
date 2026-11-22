import { useId, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  Permission,
  togglePermission,
  type PermissionDefinition,
  type PermissionID,
} from "../../api/permissions";
import { CheckboxField } from "../forms/CheckboxField";
import "./permission-fields.css";

const deletePermissions = new Set<PermissionID>([
  Permission.ServerFilesDelete,
  Permission.ServerBackupsDelete,
  Permission.ServerDelete,
]);

export function ServerPermissionTree({
  catalog,
  selected,
  onChange,
  requireView = false,
  disabled = false,
}: {
  catalog: PermissionDefinition[];
  selected: PermissionID[];
  onChange: (selected: PermissionID[]) => void;
  requireView?: boolean;
  disabled?: boolean;
}) {
  const { t } = useTranslation();
  const prefix = useId();
  function tree(parent?: PermissionID): ReactNode {
    const children = catalog.filter((item) => item.requires[0] === parent);
    if (!children.length) return null;
    return (
      <ul
        className={
          parent ? "permission-tree" : "permission-tree permission-tree-root"
        }
      >
        {children.map((item) => {
          const descriptionID = `${prefix}-${item.id}`;
          const key = item.id.replaceAll(".", "_");
          const name = t(`permissionNames.${key}`, {
            defaultValue: item.label,
          });
          const description = t(`permissionLabels.${key}`, {
            defaultValue: item.label,
          });
          const hasDescription = description !== name;
          return (
            <li key={item.id} data-permission={item.id}>
              <div className="permission-tree-node">
                <CheckboxField
                  tone={deletePermissions.has(item.id) ? "danger" : "default"}
                  checked={selected.includes(item.id)}
                  disabled={
                    disabled ||
                    (requireView && item.id === Permission.ServerView)
                  }
                  aria-describedby={hasDescription ? descriptionID : undefined}
                  onChange={(event) =>
                    onChange(
                      togglePermission(
                        selected,
                        item.id,
                        event.target.checked,
                        catalog,
                      ),
                    )
                  }
                >
                  {name}
                </CheckboxField>
                {hasDescription && (
                  <div
                    id={descriptionID}
                    className="permission-tree-description"
                  >
                    <span className="field-help">{description}</span>
                  </div>
                )}
              </div>
              {tree(item.id)}
            </li>
          );
        })}
      </ul>
    );
  }
  return tree();
}
