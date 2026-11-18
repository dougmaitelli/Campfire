import { fireEvent, render, screen } from "@testing-library/react";
import { expect, test, vi } from "vitest";
import { BackupMetadataActions } from "../../src/components/servers/BackupMetadataActions";
import type { ActionMenuItem } from "../../src/components/forms/ActionMenu";
import { BackupState } from "../../src/contracts/domains";
import type { Backup } from "../../src/api/backups";

vi.mock("../../src/components/forms/ActionMenu", () => ({
  ActionMenu: ({ items }: { items: ActionMenuItem[] }) => (
    <div role="menu">
      {items.map((item) => (
        <button
          key={item.label}
          role="menuitem"
          disabled={item.disabled}
          onClick={item.onSelect}
        >
          {item.label}
        </button>
      ))}
    </div>
  ),
}));

const backup: Backup = {
  id: "archive-one",
  name: "snapshot.tar.gz",
  display_name: "Saved world",
  pinned: false,
  size: 123,
  modified_at: "2026-11-17T12:00:00Z",
  local: true,
  remote: false,
  state: BackupState.Available,
};

test("create-only archive access offers creation without metadata mutations", () => {
  const create = vi.fn();
  render(
    <BackupMetadataActions
      backup={backup}
      canEdit={false}
      onCreateServer={create}
      onSaved={vi.fn()}
      onError={vi.fn()}
    />,
  );
  expect(screen.getAllByRole("menuitem")).toHaveLength(1);
  fireEvent.click(screen.getByRole("menuitem", { name: "Create from backup" }));
  expect(create).toHaveBeenCalledOnce();
  expect(screen.queryByRole("menuitem", { name: "Rename backup" })).toBeNull();
  expect(screen.queryByRole("menuitem", { name: "Pin backup" })).toBeNull();
});

test("existing server metadata menus do not acquire a creation action", () => {
  render(
    <BackupMetadataActions
      backup={backup}
      onSaved={vi.fn()}
      onError={vi.fn()}
    />,
  );
  expect(screen.getAllByRole("menuitem")).toHaveLength(2);
  expect(
    screen.queryByRole("menuitem", { name: "Create from backup" }),
  ).toBeNull();
});

test("disabled archive actions also disable creation", () => {
  render(
    <BackupMetadataActions
      backup={backup}
      disabled
      onCreateServer={vi.fn()}
      onSaved={vi.fn()}
      onError={vi.fn()}
    />,
  );
  expect(screen.getAllByRole("menuitem")).toHaveLength(3);
  for (const action of screen.getAllByRole("menuitem"))
    expect(action.hasAttribute("disabled")).toBe(true);
});
