import { ServersPage } from "../pages/ServersPage";
import { ServerOperationsPage } from "../pages/ServerOperationsPage";
import { useCurrentRoute } from "./useCurrentPage";
import { parseResourceRoute } from "./routeRegistry";

export function ServerRoute() {
  const route = useCurrentRoute();
  const { id, tab } = parseResourceRoute(route);
  let filePath: string | undefined;
  try {
    filePath =
      tab === "files" && route.split("/")[3]
        ? decodeURIComponent(route.split("/")[3]!)
        : undefined;
  } catch {
    filePath = undefined;
  }
  return id ? (
    <ServerOperationsPage
      key={id}
      serverID={id}
      tab={tab ?? "console"}
      filePath={filePath}
    />
  ) : (
    <ServersPage />
  );
}
