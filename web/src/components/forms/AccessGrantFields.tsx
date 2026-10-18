import { useState } from "react";
import type { AccessGrant, Role } from "../../api/access";
import { Button } from "./Button";
import { FormField } from "./FormField";
import { Select } from "./Select";
import { TextInput } from "./TextInput";

interface Props {
  grants: AccessGrant[];
  subjectRoles: Role[];
  accessRoles: { id: number; name: string }[];
  onChange: (grants: AccessGrant[], removedID?: number) => void;
}

export function AccessGrantFields({
  grants,
  subjectRoles,
  accessRoles,
  onChange,
}: Props) {
  const [subject, setSubject] = useState("user");
  const [username, setUsername] = useState("");
  const [roleID, setRoleID] = useState(subjectRoles[0]?.id ?? 0);
  const [accessID, setAccessID] = useState(accessRoles[0]?.id ?? 0);
  return (
    <>
      <p className="field-help">
        Grant access to individual users or everyone with a role. Changes apply
        when you save. Administrators always have access.
      </p>
      {grants.length === 0 && (
        <p className="field-help">No access grants assigned.</p>
      )}
      {grants.map((grant) => (
        <div
          className="access-member"
          key={
            grant.role_id === null
              ? `user:${grant.username}`
              : `role:${grant.role_id}`
          }
        >
          <span>
            {grant.role_id === null
              ? grant.username
              : `Role: ${grant.role_name}`}{" "}
            · {grant.access_role}
          </span>
          <Button
            type="button"
            variant="danger"
            onClick={() =>
              onChange(
                grants.filter((item) => item !== grant),
                grant.id > 0 ? grant.id : undefined,
              )
            }
          >
            Remove
          </Button>
        </div>
      ))}
      <FormField label="Grant to">
        <Select
          value={subject}
          onChange={(event) => setSubject(event.target.value)}
        >
          <option value="user">User</option>
          <option value="role">Role</option>
        </Select>
      </FormField>
      {subject === "user" ? (
        <FormField label="Existing username">
          <TextInput
            value={username}
            maxLength={64}
            autoComplete="off"
            onChange={(event) => setUsername(event.target.value)}
          />
        </FormField>
      ) : (
        <FormField label="Role">
          <Select
            value={roleID}
            onChange={(event) => setRoleID(Number(event.target.value))}
          >
            {subjectRoles.map((role) => (
              <option key={role.id} value={role.id}>
                {role.name}
              </option>
            ))}
          </Select>
        </FormField>
      )}
      {accessRoles.length > 1 && (
        <FormField label="Access level">
          <Select
            value={accessID}
            onChange={(event) => setAccessID(Number(event.target.value))}
          >
            {accessRoles.map((role) => (
              <option key={role.id} value={role.id}>
                {role.name}
              </option>
            ))}
          </Select>
        </FormField>
      )}
      <Button
        type="button"
        disabled={
          !accessID || (subject === "user" ? !username.trim() : !roleID)
        }
        onClick={() => {
          const level = accessRoles.find((role) => role.id === accessID);
          if (!level) return;
          const targetRole = subject === "role" ? roleID : null;
          const name = username.trim();
          const previous = grants.find((grant) =>
            targetRole === null
              ? grant.role_id === null &&
                grant.username.toLowerCase() === name.toLowerCase()
              : grant.role_id === targetRole,
          );
          const next: AccessGrant = {
            id: previous?.id ?? 0,
            user_id: previous?.user_id ?? null,
            username: targetRole === null ? name : "",
            role_id: targetRole,
            role_name:
              subjectRoles.find((role) => role.id === targetRole)?.name ?? "",
            access_role_id: level.id,
            access_role: level.name,
          };
          onChange([...grants.filter((grant) => grant !== previous), next]);
          setUsername("");
        }}
      >
        Add access
      </Button>
    </>
  );
}
