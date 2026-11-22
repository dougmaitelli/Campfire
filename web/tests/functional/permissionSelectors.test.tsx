import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { APITokensDrawer } from "../../src/drawers/APITokensDrawer";
import { RoleDrawer } from "../../src/drawers/RoleDrawer";
import { RoleScope } from "../../src/contracts/domains";
import * as tokens from "../../src/api/tokens";
import * as permissions from "../../src/api/permissions";
import {
  Permission,
  type PermissionDefinition,
} from "../../src/api/permissions";

const catalog: PermissionDefinition[] = [
  {
    id: Permission.ServerView,
    group: "Server",
    label: "View server",
    requires: [],
  },
  {
    id: Permission.ServerFilesRead,
    group: "Files",
    label: "Read files",
    requires: [Permission.ServerView],
  },
  {
    id: Permission.ServerFilesWrite,
    group: "Files",
    label: "Write files",
    requires: [Permission.ServerFilesRead],
  },
  {
    id: Permission.ServerFilesDelete,
    group: "Files",
    label: "Delete files",
    requires: [Permission.ServerFilesWrite],
  },
];
beforeEach(() => {
  HTMLDialogElement.prototype.showModal = function () {
    this.setAttribute("open", "");
  };
  HTMLDialogElement.prototype.close = function () {
    this.removeAttribute("open");
  };
  vi.spyOn(permissions, "permissionCatalog").mockResolvedValue(catalog);
  vi.spyOn(tokens, "listTokens").mockResolvedValue({
    tokens: [],
    scopes: Object.values(Permission),
    permissions: catalog,
  });
  vi.spyOn(tokens, "createToken").mockImplementation(
    async (name, _days, scopes) => ({
      secret: "cf_test_secret",
      token: {
        id: 1,
        name,
        scopes,
        prefix: "cf_test",
        created_at: 1,
        expires_at: 2000000000,
        last_used_at: 0,
      },
    }),
  );
});
afterEach(cleanup);

function checkbox(name: string) {
  return screen.getByRole<HTMLInputElement>("checkbox", { name });
}
async function tokenForm() {
  render(<APITokensDrawer onClose={vi.fn()} />);
  await screen.findByText("No active API tokens.");
  expect(screen.queryByRole("textbox", { name: "Name" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Create token" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Name" }), {
    target: { value: "Automation" },
  });
}
function choosePermissions() {
  fireEvent.click(checkbox("All my current permissions"));
}
async function create() {
  fireEvent.click(screen.getByRole("button", { name: "Create token" }));
  await waitFor(() => expect(tokens.createToken).toHaveBeenCalledTimes(1));
}

test("unrestricted tokens stay wildcard-only and permission selection is explicit", async () => {
  await tokenForm();
  expect(
    screen.queryByRole("group", { name: "Platform permissions" }),
  ).toBeNull();
  expect(
    screen.getByRole<HTMLInputElement>("checkbox", {
      name: "All my current permissions",
    }).checked,
  ).toBe(true);
  await create();
  expect(tokens.createToken).toHaveBeenCalledWith("Automation", 90, [
    Permission.AllAccess,
  ]);
});

test("generated token is selectable text in a warning banner with an inline copy icon", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
  await tokenForm();
  await create();
  const banner = screen.getByRole("region", { name: "New token secret" });
  expect(screen.getByRole("table", { name: "Active tokens" })).toBeTruthy();
  await waitFor(() => expect(document.activeElement).toBe(banner));
  expect(screen.queryByRole("textbox", { name: "Name" })).toBeNull();
  expect(banner.querySelector(".inline-message-warning")).not.toBeNull();
  expect(within(banner).queryByRole("textbox")).toBeNull();
  expect(within(banner).getByText("cf_test_secret").tagName).toBe("CODE");
  fireEvent.click(within(banner).getByRole("button", { name: "Copy token" }));
  await within(banner).findByRole("button", { name: "Copied" });
  expect(writeText).toHaveBeenCalledWith("cf_test_secret");
});

test("back returns to the list without creating a token", async () => {
  await tokenForm();
  fireEvent.click(screen.getByRole("button", { name: "Back" }));
  expect(screen.queryByRole("textbox", { name: "Name" })).toBeNull();
  expect(screen.getByText("No active API tokens.")).toBeTruthy();
  expect(tokens.createToken).not.toHaveBeenCalled();
});

test("creation errors keep the form open and preserve its values", async () => {
  vi.mocked(tokens.createToken).mockRejectedValueOnce(
    new Error("Unable to create token"),
  );
  await tokenForm();
  await create();
  await screen.findByText("Unable to create token");
  expect(
    screen.getByRole<HTMLInputElement>("textbox", { name: "Name" }).value,
  ).toBe("Automation");
  expect(screen.queryByRole("region", { name: "New token secret" })).toBeNull();
});

test("another token can be created without closing the drawer", async () => {
  await tokenForm();
  await create();
  await screen.findByRole("table", { name: "Active tokens" });
  fireEvent.click(screen.getByRole("button", { name: "Create token" }));
  expect(screen.queryByRole("table")).toBeNull();
  expect(screen.queryByRole("region", { name: "New token secret" })).toBeNull();
  expect(
    screen.getByRole<HTMLInputElement>("textbox", { name: "Name" }).value,
  ).toBe("");
  fireEvent.click(screen.getByRole("button", { name: "Back" }));
  expect(document.activeElement).toBe(
    screen.getByRole("region", { name: "New token secret" }),
  );
  expect(screen.getByRole("table")).toBeTruthy();
});

test("platform-only token does not acquire server access", async () => {
  await tokenForm();
  choosePermissions();
  const platform = screen.getByRole("group", { name: "Platform permissions" });
  const server = screen.getByRole("group", { name: "Server permissions" });
  expect(
    within(platform).getByRole("checkbox", { name: "Manage Sparks" }),
  ).toBeTruthy();
  expect(
    within(platform).queryByRole("checkbox", { name: "View server" }),
  ).toBeNull();
  expect(
    within(server).getByRole("checkbox", { name: "View server" }),
  ).toBeTruthy();
  fireEvent.click(checkbox("View audit log"));
  expect(checkbox("View server").checked).toBe(false);
  expect(checkbox("View server").disabled).toBe(false);
  await create();
  expect(tokens.createToken).toHaveBeenCalledWith("Automation", 90, [
    Permission.AuditView,
  ]);
});

test("server tree adds prerequisites and clearing a parent preserves platform scopes", async () => {
  await tokenForm();
  choosePermissions();
  fireEvent.click(checkbox("Manage Sparks"));
  fireEvent.click(checkbox("Delete files"));
  for (const name of [
    "View server",
    "Read files",
    "Write files",
    "Delete files",
  ])
    expect(checkbox(name).checked).toBe(true);
  const leaf = checkbox("Delete files").closest("li");
  expect(
    leaf?.parentElement?.closest("li")?.getAttribute("data-permission"),
  ).toBe(Permission.ServerFilesWrite);
  expect(checkbox("Delete files").className).toContain("danger");
  fireEvent.click(checkbox("Write files"));
  expect(checkbox("Delete files").checked).toBe(false);
  expect(checkbox("Read files").checked).toBe(true);
  fireEvent.click(checkbox("View server"));
  expect(checkbox("Read files").checked).toBe(false);
  expect(checkbox("Manage Sparks").checked).toBe(true);
  await create();
  expect(tokens.createToken).toHaveBeenCalledWith("Automation", 90, [
    Permission.SparksManage,
  ]);
});

test("mixed token submits both sections with expanded server prerequisites", async () => {
  await tokenForm();
  choosePermissions();
  fireEvent.click(checkbox("Read notifications"));
  fireEvent.click(checkbox("Delete files"));
  await create();
  expect(tokens.createToken).toHaveBeenCalledWith(
    "Automation",
    90,
    [
      Permission.NotificationsRead,
      Permission.ServerView,
      Permission.ServerFilesRead,
      Permission.ServerFilesWrite,
      Permission.ServerFilesDelete,
    ].sort(),
  );
});

test("switching permission modes retains custom selection without mixing wildcard scopes", async () => {
  await tokenForm();
  choosePermissions();
  expect(
    screen.getByRole<HTMLButtonElement>("button", {
      name: "Create token",
    }).disabled,
  ).toBe(true);
  fireEvent.click(checkbox("Manage Sparks"));
  fireEvent.click(checkbox("All my current permissions"));
  choosePermissions();
  expect(checkbox("Manage Sparks").checked).toBe(true);
  fireEvent.click(checkbox("All my current permissions"));
  await create();
  expect(tokens.createToken).toHaveBeenCalledWith("Automation", 90, [
    Permission.AllAccess,
  ]);
});

test("server roles keep their required view permission and share the same tree", async () => {
  render(
    <RoleDrawer
      role={null}
      onClose={vi.fn()}
      onSaved={vi.fn()}
      deleting={false}
      deleteError={null}
    />,
  );
  await screen.findByRole("checkbox", { name: "Delete files" });
  expect(checkbox("View server").checked).toBe(true);
  expect(checkbox("View server").disabled).toBe(true);
  fireEvent.click(checkbox("Delete files"));
  expect(checkbox("Write files").checked).toBe(true);
  expect(checkbox("Read files").checked).toBe(true);
});

test("custom platform roles still offer only server creation", async () => {
  render(
    <RoleDrawer
      role={{
        id: 9,
        name: "Builder",
        scope: RoleScope.Platform,
        builtin: false,
        permissions: [],
      }}
      onClose={vi.fn()}
      onSaved={vi.fn()}
      deleting={false}
      deleteError={null}
    />,
  );
  await screen.findByRole("checkbox", {
    name: "Create servers on Sparks approved by an administrator",
  });
  expect(screen.getAllByRole("checkbox")).toHaveLength(1);
  expect(screen.queryByRole("checkbox", { name: "Manage users" })).toBeNull();
});

test("built-in platform roles remain read-only", async () => {
  render(
    <RoleDrawer
      role={{
        id: 1,
        name: "Admin",
        scope: RoleScope.Platform,
        builtin: true,
        permissions: [Permission.UsersManage, Permission.SparksManage],
      }}
      onClose={vi.fn()}
      onSaved={vi.fn()}
      deleting={false}
      deleteError={null}
    />,
  );
  await screen.findByRole("checkbox", { name: "Manage users" });
  expect(checkbox("Manage users").matches(":disabled")).toBe(true);
  expect(checkbox("Manage Sparks").matches(":disabled")).toBe(true);
});
