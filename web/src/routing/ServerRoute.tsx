import { ServersPage } from "../pages/ServersPage";
import { ServerOperationsPage } from "../pages/ServerOperationsPage";
import { useCurrentRoute } from "./useCurrentPage";
import { parseResourceRoute } from "./routeRegistry";

export function ServerRoute() {
  const { id, tab } = parseResourceRoute(useCurrentRoute());
  return id ? (
    <ServerOperationsPage key={id} serverID={id} tab={tab ?? "console"} />
  ) : (
    <ServersPage />
  );
}
