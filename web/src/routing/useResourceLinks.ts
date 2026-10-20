import { useAuthentication } from "../hooks/useAuthentication";
import { createResourceLinks } from "./resourceLinks";

export function useResourceLinks() {
  const { session } = useAuthentication();
  return createResourceLinks(
    (permission) =>
      session?.admin === true &&
      session.permissions?.includes(permission) === true,
  );
}
