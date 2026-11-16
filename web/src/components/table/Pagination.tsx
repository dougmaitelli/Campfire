import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { PaginationMetadata } from "../../api/pagination";
import { Button } from "../forms/Button";
import { TextInput } from "../forms/TextInput";
import "./pagination.css";

export function Pagination({
  pagination,
  onPageChange,
  loading = false,
  label,
}: {
  pagination: PaginationMetadata | undefined;
  onPageChange: (page: number) => void;
  loading?: boolean;
  label?: string;
}) {
  const { t } = useTranslation();
  const [target, setTarget] = useState("");
  if (!pagination) return null;
  const { page, total_pages: total, total_items: count } = pagination;
  if (total <= 1 && page === 1) return null;
  const numbers = [...new Set([1, total, page - 1, page, page + 1])]
    .filter((n) => n >= 1 && n <= total)
    .sort((a, b) => a - b);
  return (
    <nav
      className="pagination"
      aria-label={label ?? t("pagination.label")}
      aria-busy={loading}
    >
      <span role="status">
        {t("pagination.summary", { page, total, count })}
      </span>
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
          const n = Number(target);
          if (Number.isSafeInteger(n) && n >= 1 && n <= total) {
            onPageChange(n);
            setTarget("");
          }
        }}
      >
        <label>
          {t("pagination.goTo")}{" "}
          <TextInput
            type="number"
            min={1}
            max={Math.max(1, total)}
            step={1}
            required
            value={target}
            disabled={loading || !total}
            onChange={(event) => setTarget(event.target.value)}
          />
        </label>
        <Button disabled={loading || !total}>{t("pagination.go")}</Button>
      </form>
    </nav>
  );
}
