import { StrictMode, type ReactNode } from "react";
import { expect, test, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ServerDrawer } from "../../src/drawers/ServerDrawer";
import {
  cancelBackupImport,
  publishBackupImport,
  type BackupImport,
} from "../../src/api/backupImports";
import { BackupMode } from "../../src/api/backups";
import { saveServer, type Server } from "../../src/api/servers";
import { serverAction } from "../../src/api/runtime";
import { RuntimeState } from "../../src/contracts/domains";

vi.mock("../../src/api/servers", async () => ({
  ...(await vi.importActual("../../src/api/servers")),
  saveServer: vi.fn(),
}));
vi.mock("../../src/api/runtime", async () => ({
  ...(await vi.importActual("../../src/api/runtime")),
  serverAction: vi.fn(),
}));

vi.mock("../../src/hooks/useAuthentication", () => ({
  useAuthentication: () => ({
    session: { admin: false, user_id: 1, permissions: [] },
  }),
}));
vi.mock("../../src/api/backupImports", () => ({
  cancelBackupImport: vi.fn(async () => {}),
  publishBackupImport: vi.fn(),
}));
vi.mock("../../src/api/access", async () => ({
  ...(await vi.importActual("../../src/api/access")),
  serverAccessOptions: vi.fn(async () => ({
    owner: null,
    subject_users: [],
    roles: [],
    subject_roles: [],
    grants: [],
  })),
}));
vi.mock("../../src/components/layout/Drawer", () => ({
  Drawer: ({
    children,
    footerLeft,
    footerRight,
  }: {
    children: ReactNode;
    footerLeft: ReactNode;
    footerRight: ReactNode;
  }) => (
    <div role="dialog" aria-label="Create server">
      {children}
      {footerLeft}
      {footerRight}
    </div>
  ),
}));

const backup: BackupImport = {
  token: "upload-token",
  mode: BackupMode.Full,
  import_ember: false,
  configuration: {
    name: "Recovered server",
    description: "",
    spark_id: "",
    ember_id: "ember",
    image: "alpine:latest",
    variables: {},
    cpu_limit: 1,
    memory_mb: 1024,
    disk_mb: 0,
    ports: [],
  },
  ember: {
    id: "ember",
    schema: 1,
    name: "Test game",
    description: "",
    images: { Default: "alpine:latest" },
    startup: "./server",
    work_dir: "/home/container",
    user: "1000:1000",
    variables: [],
  },
};

test("development effect cleanup keeps the backup token alive until explicit cancellation", async () => {
  const user = userEvent.setup();
  const close = vi.fn();
  render(
    <StrictMode>
      <ServerDrawer
        backup={backup}
        server={null}
        sparks={[]}
        embers={[backup.ember]}
        servers={[]}
        s3Configured={false}
        onClose={close}
        onSaved={vi.fn()}
      />
    </StrictMode>,
  );
  await waitFor(() =>
    expect(screen.getByRole("dialog", { name: "Create server" })).toBeTruthy(),
  );
  expect(cancelBackupImport).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Cancel" }));
  expect(cancelBackupImport).toHaveBeenCalledWith("upload-token");
  expect(close).toHaveBeenCalledOnce();
});

test("retrying backup recovery saves edited settings on the already-created server", async () => {
  const user = userEvent.setup();
  const close = vi.fn();
  const created: Server = {
    ...backup.configuration,
    id: "recovered",
    spark_id: "spark",
    ember_id: "imported-ember",
    ember_name: backup.ember.name,
    ember_hash: "hash",
    ember_update_available: false,
    owner_user_id: 1,
    permissions: [],
    spark_name: "Spark",
    game_address: "localhost",
    status: RuntimeState.NotInstalled,
    created_at: "2026-11-01T00:00:00Z",
  };
  vi.mocked(saveServer).mockResolvedValue(created);
  vi.mocked(publishBackupImport)
    .mockRejectedValueOnce(new Error("Connection interrupted"))
    .mockResolvedValue("full-recovered.tar.gz");
  render(
    <ServerDrawer
      backup={backup}
      server={null}
      sparks={[
        {
          id: "spark",
          name: "Spark",
          description: "",
          game_address: "localhost",
          cpu_limit: 8,
          memory_mb: 16384,
          disk_mb: 0,
          port_allocations: [],
          disabled: false,
          connected: true,
          created_at: created.created_at,
        },
      ]}
      embers={[backup.ember]}
      servers={[]}
      s3Configured={false}
      onClose={close}
      onSaved={vi.fn()}
    />,
  );
  for (const name of [
    "Next: Resources & ports",
    "Next: Backups",
    "Next: Access",
    "Next: Review",
  ]) {
    await user.click(screen.getByRole("button", { name }));
  }
  await user.click(screen.getByRole("button", { name: "Create server" }));
  await screen.findByText("Connection interrupted");
  await user.click(screen.getByRole("button", { name: /General/ }));
  const name = screen.getByLabelText("Server name", { exact: true });
  await user.clear(name);
  await user.type(name, "Updated recovery");
  for (const label of [
    "Next: Resources & ports",
    "Next: Backups",
    "Next: Access",
    "Next: Review",
  ]) {
    await user.click(screen.getByRole("button", { name: label }));
  }
  await user.click(screen.getByRole("button", { name: "Create server" }));
  await waitFor(() => expect(close).toHaveBeenCalledOnce());
  expect(saveServer).toHaveBeenLastCalledWith(
    expect.objectContaining({
      name: "Updated recovery",
      ember_id: "imported-ember",
    }),
    created.id,
    undefined,
  );
  expect(serverAction).toHaveBeenCalledOnce();
});
