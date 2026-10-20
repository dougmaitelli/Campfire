import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { Fieldset } from "../components/forms/Fieldset";
import { ActionGroup } from "../components/layout/ActionGroup";
import { FormField } from "../components/forms/FormField";
import { formatDisplayValue } from "../utils/formatDisplayValue";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { Select } from "../components/forms/Select";
import { useEffect, useRef, useState } from "react";
import { listSparks, type Spark } from "../api/sparks";
import type { Server, ServerPort } from "../api/servers";
import {
  beginTransfer,
  changeTransfer,
  listTransfers,
  type ServerTransfer,
} from "../api/transfers";
import { serverStatus, type RuntimeStatus } from "../api/runtime";
import { bytes } from "../api/monitoring";
import { Drawer } from "../components/layout/Drawer";

const phases: Record<string, string> = {
  queued: "Preparing",
  exporting: "Preparing source archive",
  copying: "Copying archive",
  importing: "Validating and importing",
  committing: "Changing placement",
  cleanup: "Activating destination and cleaning source",
  complete: "Complete",
  cancelling: "Cancelling",
  cancelled: "Cancelled",
  failed: "Paused after an error",
};

export function ServerTransferDialog({
  server,
  onClose,
  onChanged,
}: {
  server: Server;
  onClose: () => void;
  onChanged: () => Promise<void>;
}) {
  const links = useResourceLinks();
  const [sparks, setSparks] = useState<Spark[]>([]);
  const [items, setItems] = useState<ServerTransfer[]>([]);
  const [status, setStatus] = useState<RuntimeStatus>();
  const [targetID, setTargetID] = useState("");
  const [ports, setPorts] = useState<ServerPort[]>(server.ports);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);
  const pending = useRef(false);
  const activeIDs = useRef(new Set<string>());
  useEffect(() => {
    const controller = new AbortController();
    void listSparks(controller.signal)
      .then((value) => setSparks(value))
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error ? cause.message : "Unable to load Sparks.",
          );
      });
    return () => controller.abort();
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function poll() {
      try {
        const signal = AbortSignal.any([
          controller.signal,
          AbortSignal.timeout(15000),
        ]);
        const [history, observed] = await Promise.all([
          listTransfers(server.id, signal),
          serverStatus(server.id, signal),
        ]);
        if (!controller.signal.aborted && !pending.current) {
          const finished = history.some(
            (item) => activeIDs.current.has(item.id) && !item.active,
          );
          activeIDs.current = new Set(
            history.filter((item) => item.active).map((item) => item.id),
          );
          setItems(history);
          setStatus(observed);
          setLoadError(null);
          setLoading(false);
          if (finished) await onChanged();
        }
      } catch (cause) {
        if (!controller.signal.aborted) {
          setLoadError(
            cause instanceof Error
              ? cause.message
              : "Unable to read transfer status.",
          );
          setLoading(false);
        }
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => {
          void poll();
        }, 2000);
    }
    void poll();
    return () => {
      controller.abort();
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [server.id, onChanged, revision]);
  const choices = sparks.filter(
    (spark) => spark.id !== server.spark_id && !spark.disabled,
  );
  const target = choices.find((spark) => spark.id === targetID);
  const active = items.some((item) => item.active);
  const canBegin =
    !loading &&
    !busy &&
    !active &&
    loadError === null &&
    target !== undefined &&
    status !== undefined &&
    !status.busy &&
    ["stopped", "not_installed"].includes(status.state) &&
    ports.every((port) => port.bind_ip !== "" && port.host_port > 0);
  function selectTarget(id: string) {
    setTargetID(id);
    const spark = sparks.find((item) => item.id === id);
    setPorts(
      server.ports.map((port) => {
        const allocations =
          spark?.port_allocations.filter(
            (allocation) => allocation.protocol === port.protocol,
          ) ?? [];
        const allocation =
          allocations.find(
            (item) =>
              port.host_port >= item.start && port.host_port <= item.end,
          ) ?? allocations[0];
        return {
          ...port,
          bind_ip: allocation?.bind_ip ?? "",
          host_port: allocation
            ? Math.max(
                allocation.start,
                Math.min(port.host_port, allocation.end),
              )
            : port.host_port,
        };
      }),
    );
  }
  async function submit(
    action: "begin" | "retry" | "cancel",
    item?: ServerTransfer,
  ) {
    if (pending.current || (action === "begin" && !canBegin)) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      const next =
        action === "begin"
          ? await beginTransfer(server.id, targetID, ports)
          : await changeTransfer(item!.id, action);
      if (next.active) activeIDs.current.add(next.id);
      setItems((current) => [
        next,
        ...current.filter((entry) => entry.id !== next.id),
      ]);
      setRevision((value) => value + 1);
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to update transfer.",
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={`Transfer ${server.name}`}
      description="Move game data and completed backups to another Spark. The server stays stopped throughout the transfer."
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button disabled={busy} onClick={onClose}>
          Close
        </Button>
      }
      footerRight={
        <Button
          variant="primary"
          disabled={!canBegin}
          onClick={() => {
            void submit("begin");
          }}
        >
          Transfer server
        </Button>
      }
    >
      <div className="drawer-content">
        {(error ?? loadError) && (
          <InlineMessage role="alert">{error ?? loadError}</InlineMessage>
        )}
        <p>
          Current Spark:{" "}
          <ResourceLink href={links.spark(server.spark_id)}>
            {server.spark_name}
          </ResourceLink>
          . Ownership, access, and schedules stay with the server.
        </p>
        {status !== undefined &&
          !["stopped", "not_installed"].includes(status.state) && (
            <p className="field-help">
              Stop the server before beginning a transfer.
            </p>
          )}
        <FormField label="Destination Spark">
          <Select
            id="transfer-target"
            value={targetID}
            disabled={busy || active}
            onValueChange={(selectedValue) => selectTarget(selectedValue)}
          >
            <option value="">Select a Spark</option>
            {choices.map((spark) => (
              <option key={spark.id} value={spark.id}>
                {spark.name}
                {spark.connected ? "" : " (disconnected)"}
              </option>
            ))}
          </Select>
        </FormField>
        {target && (
          <>
            <p className="field-help">
              Game address after transfer:{" "}
              {target.game_address || "Not configured"}. Existing game clients
              may need the new address and host ports.
            </p>
            {ports.map((port, index) => (
              <Fieldset
                key={`${port.container_port}-${port.protocol}-${index}`}
                disabled={busy || active}
              >
                <legend>
                  Container port {port.container_port}/{port.protocol}
                </legend>
                <FormField label="Destination interface">
                  <Select
                    id={`transfer-ip-${index}`}
                    value={port.bind_ip}
                    onValueChange={(selectedValue) =>
                      setPorts((current) =>
                        current.map((entry, i) =>
                          i === index
                            ? { ...entry, bind_ip: selectedValue }
                            : entry,
                        ),
                      )
                    }
                  >
                    <option value="">Select an interface</option>
                    {[
                      ...new Set(
                        target.port_allocations
                          .filter(
                            (allocation) =>
                              allocation.protocol === port.protocol,
                          )
                          .map((allocation) => allocation.bind_ip),
                      ),
                    ].map((ip) => (
                      <option key={ip} value={ip}>
                        {ip}
                      </option>
                    ))}
                  </Select>
                </FormField>
                <FormField
                  label="Destination host port"
                  description={
                    <>
                      Allowed ranges:{" "}
                      {formatDisplayValue(
                        target.port_allocations
                          .filter(
                            (allocation) =>
                              allocation.protocol === port.protocol &&
                              allocation.bind_ip === port.bind_ip,
                          )
                          .map(
                            (allocation) =>
                              `${allocation.start}–${allocation.end}`,
                          )
                          .join(", "),
                      )}
                    </>
                  }
                >
                  <TextInput
                    id={`transfer-port-${index}`}
                    type="number"
                    min="1"
                    max="65535"
                    required
                    value={port.host_port}
                    onChange={(event) =>
                      setPorts((current) =>
                        current.map((entry, i) =>
                          i === index
                            ? {
                                ...entry,
                                host_port: Number(event.target.value),
                              }
                            : entry,
                        ),
                      )
                    }
                  />
                </FormField>
              </Fieldset>
            ))}
          </>
        )}
        <h3>Transfer history</h3>
        {loading && <p role="status">Loading…</p>}
        {!loading && items.length === 0 && <p>No transfers yet.</p>}
        {items.map((item) => (
          <section key={item.id} aria-label={`Transfer ${item.id}`}>
            <p>
              <strong>{phases[item.phase] ?? item.phase}</strong>:{" "}
              <ResourceLink href={links.spark(item.source_spark_id)}>
                {sparks.find((spark) => spark.id === item.source_spark_id)
                  ?.name ?? item.source_spark_id}
              </ResourceLink>{" "}
              →{" "}
              <ResourceLink href={links.spark(item.target_spark_id)}>
                {sparks.find((spark) => spark.id === item.target_spark_id)
                  ?.name ?? item.target_spark_id}
              </ResourceLink>
            </p>
            {item.total_bytes > 0 && (
              <>
                <progress
                  aria-label="Archive bytes copied"
                  max={item.total_bytes}
                  value={item.transferred_bytes}
                />
                <p>
                  {bytes(item.transferred_bytes)} / {bytes(item.total_bytes)}
                </p>
              </>
            )}
            {item.error && (
              <InlineMessage role="alert">{item.error}</InlineMessage>
            )}
            {item.active && (
              <ActionGroup>
                {item.phase === "failed" && (
                  <Button
                    disabled={busy}
                    onClick={() => {
                      void submit("retry", item);
                    }}
                  >
                    Retry transfer
                  </Button>
                )}
                {item.can_cancel && (
                  <Button
                    disabled={busy}
                    onClick={() => {
                      void submit("cancel", item);
                    }}
                  >
                    Cancel transfer
                  </Button>
                )}
                {!item.can_cancel && item.phase !== "cancelling" && (
                  <p className="field-help">
                    Placement is being committed or has changed. Retry any
                    failed finalization to finish the transfer.
                  </p>
                )}
              </ActionGroup>
            )}
          </section>
        ))}
      </div>
    </Drawer>
  );
}
