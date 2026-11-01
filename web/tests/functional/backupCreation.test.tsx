import { StrictMode, type ReactNode } from "react";
import { expect, test, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ServerDrawer } from "../../src/drawers/ServerDrawer";
import {
  cancelBackupImport,
  type BackupImport,
} from "../../src/api/backupImports";
import { BackupMode } from "../../src/api/backups";

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

test("development effect cleanup keeps the backup token alive until explicit cancellation", async () => {
  const user = userEvent.setup();
  const close = vi.fn();
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
