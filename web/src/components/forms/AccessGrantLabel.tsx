import { ResourceLink } from "../navigation/ResourceLink";
import { useResourceLinks } from "../../routing/useResourceLinks";
import type { AccessGrant } from "../../api/access";

export function AccessGrantLabel({
  grant,
  showAccessRole = true,
}: {
  grant: AccessGrant;
  showAccessRole?: boolean;
}) {
  const links = useResourceLinks();
  return (
    <>
      <ResourceLink
        href={
          grant.role_id === null
            ? links.user(grant.user_id ?? 0)
            : links.role(grant.role_id)
        }
      >
        {grant.role_id === null ? grant.username : `Role: ${grant.role_name}`}
      </ResourceLink>
      {showAccessRole && (
        <>
          {" "}
          ·{" "}
          <ResourceLink href={links.role(grant.access_role_id)}>
            {grant.access_role}
          </ResourceLink>
        </>
      )}
    </>
  );
}
