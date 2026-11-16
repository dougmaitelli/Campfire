import {
  act,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import { useCallback } from "react";
import { Pagination } from "../../src/components/table/Pagination";
import { usePagination } from "../../src/hooks/usePagination";
import { usePagedCollection } from "../../src/hooks/usePagedCollection";
import { replaceCurrentRoute } from "../../src/routing/useCurrentPage";
import { pageFixture } from "../pageFixture";
import { getCurrentPage } from "../../src/routing/navigation";
import { useResourceSelection } from "../../src/routing/useResourceSelection";

beforeEach(() => replaceCurrentRoute("notifications"));

test("pagination supports previous, numbered links, next, and validated direct jumps", async () => {
  const change = vi.fn();
  const user = userEvent.setup();
  const view = render(
    <Pagination
      pagination={pageFixture([], 5, 1000).pagination}
      onPageChange={change}
    />,
  );
  await user.click(screen.getByRole("button", { name: "Previous" }));
  expect(change).toHaveBeenLastCalledWith(4);
  await user.click(screen.getByRole("button", { name: "Page 20" }));
  expect(change).toHaveBeenLastCalledWith(20);
  await user.click(screen.getByRole("button", { name: "Next" }));
  expect(change).toHaveBeenLastCalledWith(6);
  const input = screen.getByRole("spinbutton", { name: "Go to page" });
  await user.type(input, "12");
  await user.click(screen.getByRole("button", { name: "Go" }));
  expect(change).toHaveBeenLastCalledWith(12);
  expect(
    screen.getByRole("button", { name: "Page 5" }).getAttribute("aria-current"),
  ).toBe("page");
  const calls = change.mock.calls.length;
  fireEvent.change(input, { target: { value: "21" } });
  fireEvent.submit(input.closest("form")!);
  expect(change).toHaveBeenCalledTimes(calls);
  view.rerender(
    <Pagination
      pagination={pageFixture([], 1, 1000).pagination}
      onPageChange={change}
      loading
    />,
  );
  for (const button of screen.getAllByRole<HTMLButtonElement>("button"))
    expect(button.disabled).toBe(true);
});

test("single-page collections hide navigation and boundary buttons are disabled", () => {
  const view = render(
    <Pagination
      pagination={pageFixture([]).pagination}
      onPageChange={vi.fn()}
    />,
  );
  expect(screen.queryByRole("navigation")).toBeNull();
  view.rerender(
    <Pagination
      pagination={pageFixture([], 1, 51).pagination}
      onPageChange={vi.fn()}
    />,
  );
  expect(
    screen.getByRole<HTMLButtonElement>("button", { name: "Previous" })
      .disabled,
  ).toBe(true);
  view.rerender(
    <Pagination
      pagination={pageFixture([], 2, 51).pagination}
      onPageChange={vi.fn()}
    />,
  );
  expect(
    screen.getByRole<HTMLButtonElement>("button", { name: "Next" }).disabled,
  ).toBe(true);
});

test("page URLs preserve the selected route and nested drawer selection", async () => {
  expect(getCurrentPage("jobs?page=2").id).toBe("jobs");
  expect(getCurrentPage("audit?action=server.restart&page=3").id).toBe("audit");
  replaceCurrentRoute(
    "schedules/nightly?server_id=game&page=2&execution_page=3",
  );
  const schedules = [{ id: "nightly" }];
  function Selection() {
    const selection = useResourceSelection(schedules, true);
    return (
      <>
        <p>{selection.selected?.id ?? "No selection"}</p>
        <button onClick={() => selection.select(null)}>Close</button>
        <button onClick={() => selection.select(schedules[0]!)}>Open</button>
      </>
    );
  }
  render(<Selection />);
  expect(screen.getByText("nightly")).toBeDefined();
  await userEvent.setup().click(screen.getByRole("button", { name: "Close" }));
  await screen.findByText("No selection");
  expect(window.location.hash).toBe("#schedules?server_id=game&page=2");
  await userEvent.setup().click(screen.getByRole("button", { name: "Open" }));
  await screen.findByText("nightly");
  expect(window.location.hash).toBe("#schedules/nightly?server_id=game&page=2");
});

test("shared page loading replaces rows and ignores superseded responses", async () => {
  let finish!: (value: ReturnType<typeof pageFixture<string>>) => void;
  const older = new Promise<ReturnType<typeof pageFixture<string>>>(
    (resolve) => {
      finish = resolve;
    },
  );
  const requests: number[] = [];
  function List() {
    const navigation = usePagination();
    const load = useCallback(async (_signal: AbortSignal, page: number) => {
      requests.push(page);
      if (page === 2) return older;
      return pageFixture([`Page ${page} row`], page, 151);
    }, []);
    const history = usePagedCollection(load, navigation.page);
    return (
      <>
        <div>{history.items.join(",")}</div>
        <Pagination
          pagination={history.pagination}
          onPageChange={navigation.setPage}
          loading={history.loading}
        />
      </>
    );
  }
  render(<List />);
  await screen.findByText("Page 1 row");
  await userEvent.setup().click(screen.getByRole("button", { name: "Next" }));
  await waitFor(() => expect(requests).toContain(2));
  expect(screen.queryByText("Page 1 row")).toBeNull();
  await act(async () => replaceCurrentRoute("notifications?page=3"));
  await screen.findByText("Page 3 row");
  await act(async () => finish(pageFixture(["Stale row"], 2, 151)));
  expect(screen.queryByText("Stale row")).toBeNull();
  expect(screen.getByText("Page 3 row")).toBeDefined();
});

test("retained refresh callbacks use the current page and filter and stop after unmount", async () => {
  const load = vi.fn(async (_signal: AbortSignal, page: number) =>
    pageFixture([`Original page ${page}`], page, 101),
  );
  const filtered = vi.fn(async (_signal: AbortSignal, page: number) =>
    pageFixture([`Filtered page ${page}`], page, 101),
  );
  const view = renderHook(
    ({ page, loader }) => usePagedCollection(loader, page, 0),
    { initialProps: { page: 1, loader: load } },
  );
  await waitFor(() =>
    expect(view.result.current.items).toEqual(["Original page 1"]),
  );
  const refreshFromPendingAction = view.result.current.refresh;
  view.rerender({ page: 2, loader: filtered });
  await waitFor(() =>
    expect(view.result.current.items).toEqual(["Filtered page 2"]),
  );
  expect(view.result.current.refresh).toBe(refreshFromPendingAction);
  await act(async () => refreshFromPendingAction());
  expect(load).toHaveBeenCalledTimes(1);
  expect(filtered).toHaveBeenCalledTimes(2);
  expect(filtered).toHaveBeenLastCalledWith(expect.anything(), 2);
  expect(view.result.current.items).toEqual(["Filtered page 2"]);
  expect(view.result.current.loading).toBe(false);
  view.unmount();
  await refreshFromPendingAction();
  expect(filtered).toHaveBeenCalledTimes(2);
});
