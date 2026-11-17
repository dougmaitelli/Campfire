import type { Page } from "../src/api/pagination";

export function pageFixture<T>(
  items: T[],
  page = 1,
  total = items.length,
  size = 20,
): Page<T> {
  return {
    items,
    pagination: {
      page,
      page_size: size,
      total_items: total,
      total_pages: Math.ceil(total / size),
    },
  };
}
