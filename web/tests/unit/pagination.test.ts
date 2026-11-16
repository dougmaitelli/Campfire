import { expect, test } from "vitest";
import { isPage, pageQuery } from "../../src/api/pagination";
import { pageFixture } from "../pageFixture";

const string = (value: unknown): value is string => typeof value === "string";

test("the shared reader validates the complete numbered-page contract", () => {
  expect(isPage(pageFixture(["one"]), string)).toBe(true);
  expect(isPage(pageFixture([], 3, 0), string)).toBe(true);
  expect(isPage({ items: [], next: "cursor" }, string)).toBe(false);
  expect(isPage({ ...pageFixture([1]) }, string)).toBe(false);
  for (const changes of [
    { page: 0 },
    { page: "1" },
    { page: 1.5 },
    { page_size: 0 },
    { page_size: 101 },
    { total_items: -1 },
    { total_items: 1.5 },
    { total_pages: 2 },
    { total_pages: Infinity },
  ]) {
    const value = pageFixture(["one"]);
    expect(
      isPage(
        { ...value, pagination: { ...value.pagination, ...changes } },
        string,
      ),
    ).toBe(false);
  }
  expect(isPage(pageFixture(["one", "two"], 1, 2, 1), string)).toBe(false);
});

test("all numbered history requests use the same size and query names", () => {
  expect(pageQuery(3).toString()).toBe("page=3&page_size=50");
});
