import { isRecord } from "../utils/validation";
import { apiFetch, readJSON } from "./client";
import i18n from "../i18n";

export const PAGE_SIZE = 50;
export interface PaginationMetadata {
  page: number;
  page_size: number;
  total_items: number;
  total_pages: number;
}
export interface Page<T> {
  items: T[];
  pagination: PaginationMetadata;
}
export function isPage<T>(
  value: unknown,
  validate: (item: unknown) => item is T,
): value is Page<T> {
  if (
    !isRecord(value) ||
    !Array.isArray(value.items) ||
    !value.items.every(validate) ||
    !isRecord(value.pagination)
  )
    return false;
  const p = value.pagination;
  return (
    [p.page, p.page_size, p.total_items, p.total_pages].every(
      (n) => typeof n === "number" && Number.isSafeInteger(n) && n >= 0,
    ) &&
    typeof p.page === "number" &&
    p.page >= 1 &&
    typeof p.page_size === "number" &&
    p.page_size >= 1 &&
    p.page_size <= 100 &&
    typeof p.total_items === "number" &&
    p.total_pages === Math.ceil(p.total_items / p.page_size) &&
    value.items.length <= p.page_size
  );
}
export async function readPage<T>(
  url: string,
  signal: AbortSignal,
  validate: (item: unknown) => item is T,
): Promise<Page<T>> {
  const value = await readJSON(await apiFetch(url, { signal }));
  if (!isPage(value, validate))
    throw new Error(i18n.t("pagination.unexpectedResponse"));
  return value;
}
export function pageQuery(page = 1): URLSearchParams {
  return new URLSearchParams({
    page: String(page),
    page_size: String(PAGE_SIZE),
  });
}
