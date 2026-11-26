import {
  NotificationTarget,
  NotificationKind,
  NotificationSeverity,
} from "../../src/contracts/domains";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import { pageFixture } from "../pageFixture";
import { replaceCurrentRoute } from "../../src/routing/useCurrentPage";
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
  replaceCurrentRoute("notifications");
  items = [
    {
      id: 2,
      target_type: NotificationTarget.Server,
      target_id: "survival",
      kind: NotificationKind.Recovery,
      severity: NotificationSeverity.Success,
      title: "Survival restarted",
      body: "The server is available again.",
      created_at: 1792800000,
      read_at: 0,
    },
    {
      id: 1,
      target_type: NotificationTarget.Server,
      target_id: "survival",
      kind: NotificationKind.Backup,
      severity: NotificationSeverity.Info,
      title: "Backup completed",
      body: "Your scheduled backup finished successfully.",
      created_at: 1792799900,
      read_at: 1792800000,
    },
  ];
  vi.mocked(listNotifications).mockImplementation(async () =>
    pageFixture([...items]),
  );
  vi.mocked(markNotificationRead).mockImplementation(async (id, read) => {
    items = items.map((item) =>
      item.id === id ? { ...item, read_at: read ? 1792800001 : 0 } : item,
    );
  });
  vi.mocked(markAllNotificationsRead).mockResolvedValue();
});

test("only unread rows expose an accessible acknowledgement action", async () => {
  render(<NotificationsPage />);
  const read = await screen.findByRole("button", {
    name: "Mark read: Survival restarted",
  });
  expect(screen.queryByRole("button", { name: /Mark unread:/ })).toBeNull();
  expect(
    within(
      screen.getByRole("article", { name: "Backup completed" }),
    ).queryByRole("button"),
  ).toBeNull();
  expect(read.title).toBe("Mark read");
  expect(
    screen.getByRole("button", { name: "Mark all read" }).textContent,
  ).toBe("Mark all read");
});

test.each(["{Enter}", " "])(
  "keyboard acknowledgement works with %s and cannot mark unread",
  async (key) => {
    const user = userEvent.setup();
    render(<NotificationsPage />);
    const read = await screen.findByRole("button", {
      name: "Mark read: Survival restarted",
    });
    read.focus();
    await user.keyboard(key);
    expect(markNotificationRead).toHaveBeenLastCalledWith(2, true);
    const message = screen.getByRole("article", { name: "Survival restarted" });
    await waitFor(() =>
      expect(within(message).queryByText("Unread")).toBeNull(),
    );
    expect(within(message).queryByRole("button")).toBeNull();
    await user.click(message);
    expect(markNotificationRead).toHaveBeenCalledTimes(1);
  },
);

test("row clicks acknowledge unread messages but links and text selection do not", async () => {
  const user = userEvent.setup();
  render(<NotificationsPage />);
  const row = await screen.findByRole("article", {
    name: "Survival restarted",
  });
  await user.click(within(row).getByRole("link"));
  expect(markNotificationRead).not.toHaveBeenCalled();
  const selection = window.getSelection()!;
  const range = document.createRange();
  range.selectNodeContents(
    within(row).getByText("The server is available again."),
  );
  selection.removeAllRanges();
  selection.addRange(range);
  // Dispatch directly so the test preserves the selected text.
  row.click();
  expect(markNotificationRead).not.toHaveBeenCalled();
  selection.removeAllRanges();
  await user.click(within(row).getByText("The server is available again."));
  expect(markNotificationRead).toHaveBeenCalledExactlyOnceWith(2, true);
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
  await userEvent
    .setup()
    .click(screen.getByRole("article", { name: "Survival restarted" }));
  expect(markNotificationRead).toHaveBeenCalledTimes(1);
  await act(async () => {
    complete();
  });
  expect(
    screen.getByRole<HTMLButtonElement>("button", {
      name: "Mark read: Survival restarted",
    }).disabled,
  ).toBe(false);
});

test("a failed update keeps the notification unread and shows the action error", async () => {
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
    screen.getByRole<HTMLButtonElement>("button", {
      name: "Mark read: Survival restarted",
    }).disabled,
  ).toBe(false);
  expect(
    within(
      screen.getByRole("article", { name: "Survival restarted" }),
    ).getByText("Unread"),
  ).toBeDefined();
});

test("mark all read from a direct-linked later page uses the newest inbox watermark", async () => {
  replaceCurrentRoute("notifications?page=2");
  items = Array.from({ length: 101 }, (_, index) => ({
    ...items[0]!,
    id: 101 - index,
    title: `Message ${101 - index}`,
  }));
  vi.mocked(listNotifications).mockImplementation(async ({ page = 1 }) =>
    pageFixture(items.slice((page - 1) * 20, page * 20), page, items.length),
  );
  vi.mocked(markAllNotificationsRead).mockImplementation(async (through) => {
    items = [
      { ...items[0]!, id: 102, title: "New arrival" },
      ...items.map((item) => ({
        ...item,
        read_at: item.id <= through ? 1792800001 : item.read_at,
      })),
    ];
  });
  render(<NotificationsPage />);
  await screen.findByRole("article", { name: "Message 81" });
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: "Mark all read" }));
  await waitFor(() =>
    expect(markAllNotificationsRead).toHaveBeenCalledWith(101),
  );
  await waitFor(() =>
    expect(listNotifications).toHaveBeenLastCalledWith({
      signal: expect.anything(),
      page: 2,
      pageSize: 20,
    }),
  );
  expect(items.filter((item) => !item.read_at).map((item) => item.id)).toEqual([
    102,
  ]);
  expect(window.location.hash).toBe("#notifications?page=2");
  expect(screen.queryByRole("article", { name: "Message 101" })).toBeNull();
});

test("an empty newest page never sends the unbounded zero watermark", async () => {
  replaceCurrentRoute("notifications?page=2");
  vi.mocked(listNotifications).mockImplementation(async ({ page = 1 }) =>
    pageFixture(page === 1 ? [] : items, page, page === 1 ? 0 : 52),
  );
  render(<NotificationsPage />);
  await screen.findByRole("article", { name: "Survival restarted" });
  await userEvent
    .setup()
    .click(screen.getByRole("button", { name: "Mark all read" }));
  await waitFor(() => expect(listNotifications).toHaveBeenCalledTimes(3));
  expect(markAllNotificationsRead).not.toHaveBeenCalled();
});

test("a notification action completing after navigation refreshes only the current page", async () => {
  let complete!: () => void;
  vi.mocked(markNotificationRead).mockImplementationOnce(
    () =>
      new Promise<void>((resolve) => {
        complete = resolve;
      }),
  );
  vi.mocked(listNotifications).mockImplementation(async ({ page = 1 }) =>
    pageFixture([{ ...items[0]!, title: `Page ${page} message` }], page, 51),
  );
  render(<NotificationsPage />);
  await userEvent
    .setup()
    .click(
      await screen.findByRole("button", { name: "Mark read: Page 1 message" }),
    );
  await act(async () => replaceCurrentRoute("notifications?page=2"));
  await screen.findByRole("article", { name: "Page 2 message" });
  vi.mocked(listNotifications).mockClear();
  await act(async () => complete());
  await waitFor(() => expect(listNotifications).toHaveBeenCalledTimes(1));
  expect(listNotifications).toHaveBeenLastCalledWith({
    signal: expect.anything(),
    page: 2,
    pageSize: 20,
  });
  expect(screen.getByRole("article", { name: "Page 2 message" })).toBeDefined();
});
