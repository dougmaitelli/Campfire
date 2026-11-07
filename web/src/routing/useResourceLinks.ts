import { useAuthentication } from "../hooks/useAuthentication";
import { createResourceLinks } from "./resourceLinks";
import { canAccessAdminResource } from "../permissions";

export function useResourceLinks() {
  const { session } = useAuthentication();
  return createResourceLinks((operation) =>
    canAccessAdminResource(session, operation),
  );
}
