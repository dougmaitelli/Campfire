import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { ContentPanel } from "../components/layout/ContentPanel";
import { ListToolbar } from "../components/layout/ListToolbar";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { PageHeader } from "../components/layout/PageHeader";
import { useEffect, useState } from "react";
import { activityCursor, listAudit, type AuditEvent } from "../api/activity";

import { Table } from "../components/table/Table";
import { AuditOutcome } from "../components/activity/AuditOutcome";

export function AuditPage() {
  const links = useResourceLinks();
  const [items, setItems] = useState<AuditEvent[]>([]);
  const [action, setAction] = useState("");
  const [target, setTarget] = useState("");
  const [filters, setFilters] = useState({ action: "", target: "" });
  const [version, setVersion] = useState(0);
  const [busy, setBusy] = useState(false);
  const [older, setOlder] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    const controller = new AbortController();
    void listAudit(controller.signal, "", filters.action, filters.target)
      .then((next) => {
        if (!controller.signal.aborted) {
          setItems(next);
          setOlder(next.length === 100);
          setError(null);
        }
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : "Unable to load audit records.",
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [filters, version]);
  async function more() {
    if (busy || !items.length) return;
    setBusy(true);
    try {
      const next = await listAudit(
        AbortSignal.timeout(15000),
        activityCursor(items.at(-1)),
        filters.action,
        filters.target,
      );
      setItems((current) => [...current, ...next]);
      setOlder(next.length === 100);
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to load audit records.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <section aria-labelledby="page-title">
      <PageHeader
        title="Audit"
        description="Administrative changes, authentication attempts, and server operations."
        actions={
          <>
            <Button
              onClick={() => {
                setLoading(true);
                setVersion((v) => v + 1);
              }}
            >
              Refresh
            </Button>
          </>
        }
      />
      {error && items.length > 0 && (
        <InlineMessage role="alert">{error}</InlineMessage>
      )}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          setLoading(true);
          setFilters({ action: action.trim(), target: target.trim() });
        }}
      >
        <ListToolbar>
          <FormField label="Action">
            <TextInput
              value={action}
              maxLength={100}
              placeholder="server.restart"
              onChange={(e) => setAction(e.target.value)}
            />
          </FormField>
          <FormField label="Target">
            <TextInput
              value={target}
              maxLength={150}
              placeholder="server:ID"
              onChange={(e) => setTarget(e.target.value)}
            />
          </FormField>
          <Button>Filter</Button>
        </ListToolbar>
      </form>
      <ContentPanel>
        <Table
          empty={items.length === 0}
          loading={loading}
          error={error}
          resource="audit records"
          emptyMessage="No audit records"
          onRetry={() => {
            setLoading(true);
            setVersion((value) => value + 1);
          }}
        >
          <caption className="visually-hidden">Audit records</caption>
          <thead>
            <tr>
              <th>Time</th>
              <th>Actor</th>
              <th>Action</th>
              <th>Target</th>
              <th>Outcome</th>
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
                    {item.username || "Unauthenticated"}
                  </ResourceLink>
                  {item.api_token_id
                    ? ` (API token #${item.api_token_id})`
                    : ""}
                  {item.user?.email && (
                    <div className="field-help">{item.user.email}</div>
                  )}
                  {item.user?.deleted_at && (
                    <div className="field-help">Deleted account</div>
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
        {older && (
          <Button
            disabled={busy}
            onClick={() => {
              void more();
            }}
          >
            Load older records
          </Button>
        )}
      </ContentPanel>
    </section>
  );
}
