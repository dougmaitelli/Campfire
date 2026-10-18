import { Button } from "../components/forms/Button";
import { lazy, Suspense, useState } from "react";
import type { Server } from "../api/servers";
import { Drawer } from "../components/layout/Drawer";
const ServerTerminal = lazy(() =>
  import("../components/servers/ServerTerminal").then((module) => ({
    default: module.ServerTerminal,
  })),
);

export function ServerConsoleDialog({
  server,
  onClose,
  embedded = false,
}: {
  server: Server;
  onClose: () => void;
  embedded?: boolean;
}) {
  const [busy, setBusy] = useState(false);
  return (
    <Drawer
      embedded={embedded}
      title="Console"
      description="Live server output and commands."
      busy={busy}
      onClose={onClose}
      footerLeft={
        embedded ? null : (
          <Button disabled={busy} onClick={onClose}>
            Close
          </Button>
        )
      }
      footerRight={null}
    >
      <div className="drawer-content">
        <Suspense fallback={<p role="status">Loading console…</p>}>
          <ServerTerminal serverID={server.id} onBusyChange={setBusy} />
        </Suspense>
      </div>
    </Drawer>
  );
}
