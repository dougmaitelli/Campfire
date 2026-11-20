import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  listStatusPages,
  deleteStatusPage,
  statusPageURL,
  type StatusPage,
} from "../api/statusPages";
import { useCollection } from "../hooks/useCollection";
import { useConfirmation } from "../hooks/useConfirmation";
import { PageHeader } from "../components/layout/PageHeader";
import { Copy, Check } from "lucide-react";
import "./status-pages.css";
import { ContentPanel } from "../components/layout/ContentPanel";
import { Table, TableRow } from "../components/table/Table";
import { InlineMessage } from "../components/status/InlineMessage";
import { StatusPageDrawer } from "../drawers/StatusPageDrawer";

export function StatusPagesPage() {
  const { t } = useTranslation();
  const pages = useCollection(listStatusPages);
  const confirmation = useConfirmation();
  const [editing, setEditing] = useState<{ page: StatusPage | null } | null>(
    null,
  );
  const [busy, setBusy] = useState(false);
  const pending = useRef(false);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState<string | null>(null);
  async function remove(page: StatusPage) {
    if (pending.current) return;
    pending.current = true;
    try {
      if (
        !(await confirmation.confirm({
          title: t("statusPages.deleteTitle"),
          description: t("statusPages.deleteDescription", {
            title: page.title,
          }),
          confirmLabel: t("common.delete"),
          danger: true,
        }))
      )
        return;
      setBusy(true);
      setError(null);
      await deleteStatusPage(page.id);
      pages.remove(page.id);
      setEditing(null);
      setCopied((current) => (current === page.id ? null : current));
      await pages.refresh();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : t("statusPages.failed"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  async function copy(page: StatusPage) {
    try {
      await navigator.clipboard.writeText(
        new URL(statusPageURL(page.slug), window.location.origin).href,
      );
      setCopied(page.id);
      setError(null);
    } catch {
      setError(t("statusPages.copyFailed"));
    }
  }
  function edit(page: StatusPage) {
    setError(null);
    setEditing({ page });
  }
  const selectedPage = editing?.page;
  const copiedPage = pages.items.find((page) => page.id === copied);
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title={t("common.statusPages")}
        description={t("statusPages.description")}
        primaryAction={{
          label: t("statusPages.create"),
          compactLabel: t("common.createAction"),
          disabled: busy,
          onClick: () => {
            setError(null);
            setEditing({ page: null });
          },
        }}
      />
      {(error ?? pages.error) && (
        <InlineMessage role="alert">{error ?? pages.error}</InlineMessage>
      )}
      <span className="visually-hidden" role="status">
        {copiedPage ? t("statusPages.copied", { title: copiedPage.title }) : ""}
      </span>
      <ContentPanel>
        <Table
          empty={pages.items.length === 0}
          loading={pages.loading}
          error={pages.error}
          onRetry={() => void pages.refresh()}
          resource={t("common.statusPages")}
          emptyMessage={t("statusPages.noPages")}
        >
          <caption className="visually-hidden">
            {t("common.statusPages")}
          </caption>
          <thead>
            <tr>
              <th>{t("statusPages.title")}</th>
              <th>{t("statusPages.url")}</th>
              <th>{t("common.servers")}</th>
            </tr>
          </thead>
          <tbody>
            {pages.items.map((page) => (
              <TableRow
                key={page.id}
                onOpen={busy ? undefined : () => edit(page)}
              >
                <td>
                  <button
                    type="button"
                    className="table-row-trigger"
                    disabled={busy}
                    aria-label={t("statusPages.editPage", {
                      title: page.title,
                    })}
                    onClick={() => edit(page)}
                  >
                    {page.title}
                  </button>
                </td>
                <td>
                  <span className="status-page-url">
                    <a
                      href={statusPageURL(page.slug)}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {statusPageURL(page.slug)}
                    </a>
                    <button
                      type="button"
                      className="status-page-copy"
                      disabled={busy}
                      aria-label={t(
                        copied === page.id
                          ? "statusPages.copied"
                          : "statusPages.copyURL",
                        { title: page.title },
                      )}
                      title={t(
                        copied === page.id
                          ? "statusPages.copied"
                          : "statusPages.copyURL",
                        { title: page.title },
                      )}
                      onClick={() => void copy(page)}
                    >
                      {copied === page.id ? (
                        <Check size="1em" aria-hidden="true" />
                      ) : (
                        <Copy size="1em" aria-hidden="true" />
                      )}
                    </button>
                  </span>
                </td>
                <td>{page.entries.length}</td>
              </TableRow>
            ))}
          </tbody>
        </Table>
      </ContentPanel>
      {editing && (
        <StatusPageDrawer
          page={editing.page}
          onDelete={selectedPage ? () => void remove(selectedPage) : undefined}
          deleting={busy}
          deleteError={error}
          onClose={() => setEditing(null)}
          onSaved={(page) => {
            pages.upsert(page);
            setEditing(null);
            setCopied(null);
          }}
        />
      )}
      {confirmation.dialog}
    </section>
  );
}
