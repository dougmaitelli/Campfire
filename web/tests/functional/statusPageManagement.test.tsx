import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { StatusPagesPage } from "../../src/pages/StatusPagesPage";
import type { StatusPage } from "../../src/api/statusPages";
import { useCollection } from "../../src/hooks/useCollection";

const publication: StatusPage = {
  id: "page",
  title: "Community",
  slug: "community",
  entries: [],
};
const other: StatusPage = {
  id: "other",
  title: "Other",
  slug: "other",
  entries: [],
};

beforeEach(() => {
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
    configurable: true,
    value: function (this: HTMLDialogElement) {
      this.setAttribute("open", "");
    },
  });
  Object.defineProperty(HTMLDialogElement.prototype, "close", {
    configurable: true,
    value: function (this: HTMLDialogElement) {
      this.removeAttribute("open");
    },
  });
});

async function confirmDeletion() {
  const link = await screen.findByRole("link", { name: "/status/community" });
  const row = link.closest("tr");
  if (!row) throw new Error("publication row missing");
  expect(within(row).queryByRole("button", { name: "Delete" })).toBeNull();
  fireEvent.click(row);
  const editor = await screen.findByRole("dialog", {
    name: "Edit status page",
  });
  await waitFor(() =>
    expect(
      within(editor)
        .getByRole("button", { name: "Add server" })
        .hasAttribute("disabled"),
    ).toBe(true),
  );
  fireEvent.click(within(editor).getByRole("button", { name: "Delete" }));
  fireEvent.click(
    within(
      screen.getByRole("dialog", { name: "Delete status page" }),
    ).getByRole("button", {
      name: "Delete",
    }),
  );
}

test("confirmed deletion is retained when the follow-up collection refresh fails", async () => {
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValueOnce(Response.json([publication, other]))
    .mockResolvedValueOnce(Response.json([]))
    .mockResolvedValueOnce(new Response(null, { status: 204 }))
    .mockResolvedValueOnce(
      Response.json({ error: "Refresh unavailable" }, { status: 503 }),
    );
  render(<StatusPagesPage />);
  await confirmDeletion();
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(4));
  await act(async () => {});
  expect(screen.queryByRole("link", { name: "/status/community" })).toBeNull();
  expect(screen.queryByText("Community", { exact: true })).toBeNull();
  expect(screen.getByRole("link", { name: "/status/other" })).toBeTruthy();
  expect(screen.getAllByText("Refresh unavailable.").length).toBeGreaterThan(0);
});

test("removing a confirmed deletion cancels reads that could restore it", async () => {
  let finish: (pages: StatusPage[]) => void = () => {
    throw new Error("refresh not started");
  };
  let activeSignal: AbortSignal | undefined;
  const list = vi
    .fn<(signal: AbortSignal) => Promise<StatusPage[]>>()
    .mockResolvedValueOnce([publication, other])
    .mockImplementationOnce((signal) => {
      activeSignal = signal;
      return new Promise((resolve) => {
        finish = resolve;
      });
    });
  const { result } = renderHook(() => useCollection(list));
  await waitFor(() => expect(result.current.loading).toBe(false));
  act(() => {
    void result.current.refresh();
  });
  act(() => {
    result.current.remove(publication.id);
  });
  expect(activeSignal?.aborted).toBe(true);
  await act(async () => {
    finish([publication, other]);
  });
  expect(result.current.items).toEqual([other]);
});

test("a rejected deletion preserves the page and does not refresh the collection", async () => {
  const fetch = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValueOnce(Response.json([publication]))
    .mockResolvedValueOnce(Response.json([]))
    .mockResolvedValueOnce(
      Response.json({ error: "Deletion rejected" }, { status: 403 }),
    );
  render(<StatusPagesPage />);
  await confirmDeletion();
  await within(
    screen.getByRole("dialog", { name: "Edit status page" }),
  ).findByText("Deletion rejected.");
  expect(fetch).toHaveBeenCalledTimes(3);
  expect(screen.getByRole("link", { name: "/status/community" })).toBeTruthy();
  expect(screen.getByRole("button", { name: "Edit Community" })).toBeTruthy();
});
