import type { Server } from "../../api/servers";
import { JobList } from "../jobs/JobList";

export function ServerJobs({
  server,
  onChanged,
}: {
  server: Server;
  onChanged?: (() => Promise<void>) | undefined;
}) {
  return <JobList key={server.id} serverID={server.id} onChanged={onChanged} />;
}
