import {
  act,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { beforeEach, expect, test, vi } from "vitest";
import { Pagination } from "../../src/components/table/Pagination";
import { usePagination } from "../../src/hooks/usePagination";
import { replaceCurrentRoute } from "../../src/routing/useCurrentPage";
import { pageFixture } from "../pageFixture";

vi.mock("../../src/components/forms/Select", () => ({
  Select: ({
    onValueChange,
    ...props
  }: ComponentProps<"select"> & { onValueChange: (value: string) => void }) => (
    <select
      {...props}
      onChange={(event) => onValueChange(event.target.value)}
    />
  ),
}));

beforeEach(() => replaceCurrentRoute("jobs"));

test("page-size selection offers 20, 50, and 100 and remains available on a single page", async () => {
  const user = userEvent.setup();
  const change = vi.fn();
  const view = render(
    <Pagination
      pagination={pageFixture([], 1, 15).pagination}
      onPageChange={vi.fn()}
      onPageSizeChange={change}
    />,
  );
  expect(
    screen.getAllByRole("option").map((option) => option.textContent),
  ).toEqual(["20", "50", "100"]);
  await user.selectOptions(
    screen.getByRole("combobox", { name: "Rows per page" }),
    "50",
  );
  expect(change).toHaveBeenLastCalledWith(50);
  view.rerender(
    <Pagination
      pagination={pageFixture([], 1, 15, 50).pagination}
      onPageChange={vi.fn()}
      onPageSizeChange={change}
      loading
    />,
  );
  expect(
    screen.getByRole<HTMLSelectElement>("combobox", { name: "Rows per page" })
      .disabled,
  ).toBe(true);
});

test("changing page size resets only its list and preserves filters and drawer selection", async () => {
  replaceCurrentRoute(
    "schedules/nightly?server_id=game&page=3&page_size=50&execution_page=2&execution_page_size=100",
  );
  const { result } = renderHook(() => ({
    parent: usePagination(),
    nested: usePagination("execution_page"),
  }));
  expect(result.current.parent.pageSize).toBe(50);
  expect(result.current.nested.pageSize).toBe(100);
  act(() => result.current.nested.setPageSize(50));
  await waitFor(() => expect(result.current.nested.page).toBe(1));
  expect(result.current.parent.page).toBe(3);
  expect(result.current.nested.pageSize).toBe(50);
  expect(window.location.hash).toBe(
    "#schedules/nightly?server_id=game&page=3&page_size=50&execution_page_size=50",
  );
  act(() => result.current.parent.setPageSize(20));
  await waitFor(() => expect(result.current.parent.pageSize).toBe(20));
  expect(result.current.parent.page).toBe(1);
  expect(result.current.nested.pageSize).toBe(50);
  expect(window.location.hash).toBe(
    "#schedules/nightly?server_id=game&execution_page_size=50",
  );
  act(() =>
    replaceCurrentRoute(
      "schedules/nightly?server_id=game&page=3&page_size=50&execution_page=2&execution_page_size=100",
    ),
  );
  await waitFor(() => expect(result.current.parent.pageSize).toBe(50));
  expect(result.current.parent.page).toBe(3);
  expect(result.current.nested.page).toBe(2);
  expect(result.current.nested.pageSize).toBe(100);
});

test.each(["0", "101", "-1", "1.5", "bad"])(
  "invalid page size %s defaults to 20",
  (size) => {
    replaceCurrentRoute(`jobs?page_size=${size}`);
    const { result } = renderHook(() => usePagination());
    expect(result.current.pageSize).toBe(20);
  },
);
