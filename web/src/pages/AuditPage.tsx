import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { listAudit } from "../api/activity";
import { usePagination } from "../hooks/usePagination";
import { usePagedCollection } from "../hooks/usePagedCollection";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { ContentPanel } from "../components/layout/ContentPanel";
import { ListToolbar } from "../components/layout/ListToolbar";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { PageHeader } from "../components/layout/PageHeader";
import { Table } from "../components/table/Table";
import { Pagination } from "../components/table/Pagination";
import { AuditOutcome } from "../components/activity/AuditOutcome";

export function AuditPage() {
  const { t } = useTranslation();
  const links = useResourceLinks();
  const navigation = usePagination();
  const actionFilter = navigation.query.get("action") ?? "";
  const targetFilter = navigation.query.get("target") ?? "";
  const signature = JSON.stringify([actionFilter, targetFilter]);
  const [draft, setDraft] = useState({
    signature,
    action: actionFilter,
    target: targetFilter,
  });
  // Discard the previous URL's draft when navigating, including Back/Forward.
  // Otherwise returning to its signature resurrects an already-submitted edit.
  if (draft.signature !== signature) {
    setDraft({ signature, action: actionFilter, target: targetFilter });
  }
  const action = draft.signature === signature ? draft.action : actionFilter;
  const target = draft.signature === signature ? draft.target : targetFilter;
  const load = useCallback(
    (signal: AbortSignal, page: number) =>
      listAudit(signal, page, actionFilter, targetFilter),
    [actionFilter, targetFilter],
  );
  const history = usePagedCollection(load, navigation.page);
  const { items, loading, error } = history;
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title={t("common.audit")}
        description={t(
          "auditPage.administrativeChangesAuthenticationAttemptsAndServerOperations",
        )}
      />
      {error && items.length > 0 && (
        <InlineMessage role="alert">{error}</InlineMessage>
      )}
      <form
        onSubmit={(event) => {
          event.preventDefault();
          navigation.update({
            action: action.trim(),
            target: target.trim(),
            page: null,
          });
        }}
      >
        <ListToolbar>
          <FormField label={t("auditPage.action")}>
            <TextInput
              value={action}
              maxLength={100}
              placeholder={t("auditPage.serverRestart")}
              onChange={(event) =>
                setDraft({ signature, action: event.target.value, target })
              }
            />
          </FormField>
          <FormField label={t("auditPage.target")}>
            <TextInput
              value={target}
              maxLength={150}
              placeholder={t("auditPage.serverID")}
              onChange={(event) =>
                setDraft({ signature, action, target: event.target.value })
              }
            />
          </FormField>
          <Button>{t("auditPage.filter")}</Button>
        </ListToolbar>
      </form>
      <ContentPanel>
        <Table
          empty={items.length === 0}
          loading={loading}
          error={error}
          resource={t("auditPage.auditRecords")}
          emptyMessage={t("auditPage.noAuditRecords")}
          onRetry={() => void history.refresh()}
        >
          <caption className="visually-hidden">
            {t("auditPage.auditRecords2")}
          </caption>
          <thead>
            <tr>
              <th>{t("auditPage.time")}</th>
              <th>{t("auditPage.actor")}</th>
              <th>{t("auditPage.action")}</th>
              <th>{t("auditPage.target")}</th>
              <th>{t("auditPage.outcome")}</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.id}>
                <td>{new Date(item.created_at * 1000).toLocaleString()}</td>
                <td>
                  <ResourceLink
                    href={
                      item.user_id && !item.user?.deleted_at
                        ? links.user(item.user_id)
                        : undefined
                    }
                  >
                    {item.username || t("auditPage.unauthenticated")}
                  </ResourceLink>
                  {item.api_token_id
                    ? t("auditPage.apiTokenReference", {
                        id: item.api_token_id,
                      })
                    : ""}
                  {item.user?.email && (
                    <div className="field-help">{item.user.email}</div>
                  )}
                  {item.user?.deleted_at && (
                    <div className="field-help">
                      {t("auditPage.deletedAccount")}
                    </div>
                  )}
                </td>
                <td>{item.action}</td>
                <td>
                  <ResourceLink href={links.target(item.target)}>
                    {item.target}
                  </ResourceLink>
                </td>
                <td>
                  <AuditOutcome outcome={item.outcome} />
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        <Pagination
          pagination={history.pagination}
          onPageChange={navigation.setPage}
          loading={loading}
        />
      </ContentPanel>
    </section>
  );
}
