import { useTranslation } from "react-i18next";
import { PAGE_SIZES, type PaginationMetadata } from "../../api/pagination";
import { Button } from "../forms/Button";
import { TextInput } from "../forms/TextInput";
import { Select } from "../forms/Select";
import "./pagination.css";

export function Pagination({
  pagination,
  onPageChange,
  onPageSizeChange,
  loading = false,
  label,
}: {
  pagination: PaginationMetadata | undefined;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: number) => void;
  loading?: boolean;
  label?: string;
}) {
  const { t } = useTranslation();
  if (!pagination) return null;
  const {
    page,
    page_size: size,
    total_pages: total,
    total_items: count,
  } = pagination;
  if (!count && page === 1) return null;
  const offset = (page - 1) * size;
  const start = offset < count ? offset + 1 : 0;
  const end = offset < count ? Math.min(offset + size, count) : 0;
  const numbers = [...new Set([1, total, page - 1, page, page + 1])]
    .filter((n) => n >= 1 && n <= total)
    .sort((a, b) => a - b);
  return (
    <nav
      className="pagination"
      aria-label={label ?? t("pagination.label")}
      aria-busy={loading}
    >
      <div className="pagination-pages">
        <Button
          disabled={loading || page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          {t("pagination.previous")}
        </Button>
        {numbers.map((n, index) => (
          <span className="pagination-page" key={n}>
            {index > 0 && n > numbers[index - 1]! + 1 && (
              <span aria-hidden="true">…</span>
            )}
            <Button
              aria-label={t("pagination.page", { page: n })}
              aria-current={n === page ? "page" : undefined}
              disabled={loading || n === page}
              onClick={() => onPageChange(n)}
            >
              {n}
            </Button>
          </span>
        ))}
        <Button
          disabled={loading || page >= total}
          onClick={() => onPageChange(page + 1)}
        >
          {t("pagination.next")}
        </Button>
      </div>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          const n = Number(new FormData(event.currentTarget).get("page"));
          if (Number.isSafeInteger(n) && n >= 1 && n <= total) {
            onPageChange(n);
          }
        }}
      >
        <label>
          {t("pagination.goTo")}{" "}
          <TextInput
            key={`${page}:${size}`}
            name="page"
            type="number"
            min={1}
            max={Math.max(1, total)}
            step={1}
            required
            defaultValue={page}
            disabled={loading || !total}
          />
        </label>
        <Button disabled={loading || !total}>{t("pagination.go")}</Button>
      </form>
      <div className="pagination-meta">
        <label>
          {t("pagination.pageSize")}
          <Select
            aria-label={t("pagination.pageSize")}
            value={size}
            disabled={loading}
            onValueChange={(value) => onPageSizeChange(Number(value))}
          >
            {[...new Set([...PAGE_SIZES, size])]
              .sort((a, b) => a - b)
              .map((value) => (
                <option key={value} value={value}>
                  {value}
                </option>
              ))}
          </Select>
        </label>
        <span className="pagination-summary" role="status">
          {t("pagination.summary", { start, end, count })}
        </span>
      </div>
    </nav>
  );
}
