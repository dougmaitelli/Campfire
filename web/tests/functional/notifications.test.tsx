import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import { NotificationsPage } from "../../src/pages/NotificationsPage";
import type * as NotificationsAPI from "../../src/api/notifications";
import {
  listNotifications,
  markNotificationRead,
  markAllNotificationsRead,
  type Notification,
} from "../../src/api/notifications";

vi.mock("../../src/hooks/useAuthentication", () => ({
  useAuthentication: () => ({ session: null }),
}));
vi.mock("../../src/api/notifications", async (importOriginal) => ({
  ...(await importOriginal<typeof NotificationsAPI>()),
  listNotifications: vi.fn(),
  markNotificationRead: vi.fn(),
  markAllNotificationsRead: vi.fn(),
}));

let items: Notification[];
beforeEach(() => {
  vi.resetAllMocks();
  items = [
    {
      id: 2,
      target_type: "server",
      target_id: "survival",
      kind: "recovery",
      severity: "success",
      title: "Survival restarted",
      body: "The server is available again.",
      created_at: 1792800000,
      read_at: 0,
    },
    {
      id: 1,
      target_type: "server",
      target_id: "survival",
      kind: "backup",
      severity: "info",
      title: "Backup completed",
      body: "Your scheduled backup finished successfully.",
      created_at: 1792799900,
      read_at: 1792800000,
    },
  ];
  vi.mocked(listNotifications).mockImplementation(async () => [...items]);
  vi.mocked(markNotificationRead).mockImplementation(async (id, read) => {
    items = items.map((item) =>
      item.id === id ? { ...item, read_at: read ? 1792800001 : 0 } : item,
    );
  });
  vi.mocked(markAllNotificationsRead).mockResolvedValue();
});

test("read and unread actions use envelope icons with tooltips and accessible names", async () => {
  render(<NotificationsPage />);
  const read = await screen.findByRole("button", {
    name: "Mark read: Survival restarted",
  });
  const unread = screen.getByRole("button", {
    name: "Mark unread: Backup completed",
  });
  expect(read.classList.contains("button-icon")).toBe(true);
  expect(unread.classList.contains("button-icon")).toBe(true);
  expect(read.textContent).toBe("");
  expect(unread.textContent).toBe("");
  expect(read.title).toBe("Mark read");
  expect(unread.title).toBe("Mark unread");
  expect(
    read.querySelector("svg.lucide-mail-open")?.getAttribute("aria-hidden"),
  ).toBe("true");
  expect(
    unread.querySelector("svg.lucide-mail")?.getAttribute("aria-hidden"),
  ).toBe("true");
  expect(
    screen.getByRole("button", { name: "Mark all read" }).textContent,
  ).toBe("Mark all read");
});

test("keyboard and pointer activation update the notification and switch its action icon", async () => {
  const user = userEvent.setup();
  render(<NotificationsPage />);
  const read = await screen.findByRole("button", {
    name: "Mark read: Survival restarted",
  });
  read.focus();
  await user.keyboard("{Enter}");
  expect(markNotificationRead).toHaveBeenLastCalledWith(2, true);
  const unread = await screen.findByRole("button", {
    name: "Mark unread: Survival restarted",
  });
  expect(unread.querySelector("svg.lucide-mail")).not.toBeNull();
  const message = screen.getByRole("article", { name: "Survival restarted" });
  expect(within(message).queryByText("Unread")).toBeNull();
  await user.click(unread);
  expect(markNotificationRead).toHaveBeenLastCalledWith(2, false);
  const restored = await screen.findByRole("button", {
    name: "Mark read: Survival restarted",
  });
  expect(restored.querySelector("svg.lucide-mail-open")).not.toBeNull();
  expect(within(message).getByText("Unread")).toBeDefined();
});

test("icon actions remain disabled while a notification update is pending", async () => {
  let complete!: () => void;
  vi.mocked(markNotificationRead).mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  render(<NotificationsPage />);
  const read = await screen.findByRole("button", {
    name: "Mark read: Survival restarted",
  });
  await userEvent.setup().click(read);
  for (const button of screen.getAllByRole<HTMLButtonElement>("button"))
    expect(button.disabled).toBe(true);
  await act(async () => {
    complete();
  });
  expect(
    screen.getByRole<HTMLButtonElement>("button", {
      name: "Mark read: Survival restarted",
    }).disabled,
  ).toBe(false);
});

test("a failed update keeps the current icon and shows the action error", async () => {
  vi.mocked(markNotificationRead).mockRejectedValueOnce(
    new Error("Unable to update notification."),
  );
  render(<NotificationsPage />);
  const read = await screen.findByRole("button", {
    name: "Mark read: Survival restarted",
  });
  await userEvent.setup().click(read);
  expect(screen.getByRole("alert").textContent).toBe(
    "Unable to update notification.",
  );
  expect(
    screen
      .getByRole("button", { name: "Mark read: Survival restarted" })
      .querySelector("svg.lucide-mail-open"),
  ).not.toBeNull();
});
