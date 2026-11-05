import { RuntimeState, JobStatus } from "../contracts/domains";
import { API_READ_TIMEOUT_MS } from "../api/client";
import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { Fieldset } from "../components/forms/Fieldset";
import { FormField } from "../components/forms/FormField";
import { formatDisplayValue } from "../utils/formatDisplayValue";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { Select } from "../components/forms/Select";
import { useEffect, useRef, useState } from "react";
import { listSparks, type Spark } from "../api/sparks";
import type { Server, ServerPort } from "../api/servers";
import { beginTransfer } from "../api/transfers";
import { listJobs, type ServerJob } from "../api/jobs";
import { serverStatus, type RuntimeStatus } from "../api/runtime";
import { Drawer } from "../components/layout/Drawer";
import { isEnabledSpark } from "../utils/sparkDestinations";

export function ServerTransferDrawer({
  server,
  onClose,
  onSubmitted,
}: {
  server: Server;
  onClose: () => void;
  onSubmitted: () => void;
}) {
  const { t } = useTranslation();
  const links = useResourceLinks();
  const [sparks, setSparks] = useState<Spark[]>([]);
  const [items, setItems] = useState<ServerJob[]>([]);
  const [status, setStatus] = useState<RuntimeStatus>();
  const [targetID, setTargetID] = useState("");
  const [ports, setPorts] = useState<ServerPort[]>(server.ports);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const pending = useRef(false);
  useEffect(() => {
    const controller = new AbortController();
    void listSparks(controller.signal)
      .then((value) => setSparks(value))
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error
              ? cause.message
              : i18n.t("serverTransferDrawer.unableToLoadSparks"),
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
          AbortSignal.timeout(API_READ_TIMEOUT_MS),
        ]);
        const [history, observed] = await Promise.all([
          listJobs(server.id, signal),
          serverStatus(server.id, signal),
        ]);
        if (!controller.signal.aborted && !pending.current) {
          setItems(history.items);
          setStatus(observed);
          setLoadError(null);
          setLoading(false);
        }
      } catch (cause) {
        if (!controller.signal.aborted) {
          setLoadError(
            cause instanceof Error
              ? cause.message
              : i18n.t("serverTransferDrawer.unableToReadTransferStatus"),
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
  }, [server.id]);
  const choices = sparks.filter(
    (spark) => spark.id !== server.spark_id && isEnabledSpark(spark),
  );
  const target = choices.find((spark) => spark.id === targetID);
  const active = items.some(
    (item) =>
      item.status !== JobStatus.Completed &&
      item.status !== JobStatus.Cancelled,
  );
  const canBegin =
    !loading &&
    !busy &&
    !active &&
    loadError === null &&
    target !== undefined &&
    status !== undefined &&
    !status.busy &&
    [RuntimeState.Stopped, RuntimeState.NotInstalled].some(
      (value) => value === status.state,
    ) &&
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
  async function submit() {
    if (pending.current || !canBegin) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      await beginTransfer(server.id, targetID, ports);
      onSubmitted();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("serverTransferDrawer.unableToUpdateTransfer"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={t("serverTransferDrawer.transfer", { serverName: server.name })}
      description={t(
        "serverTransferDrawer.moveGameDataAndCompletedBackupsToAnotherSparkThe",
      )}
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button disabled={busy} onClick={onClose}>
          {t("common.close")}
        </Button>
      }
      footerRight={
        <Button
          variant="primary"
          disabled={!canBegin}
          onClick={() => {
            void submit();
          }}
        >
          {t("serverTransferDrawer.transferServer")}
        </Button>
      }
    >
      <div className="panel-content">
        {(error ?? loadError) && (
          <InlineMessage role="alert">{error ?? loadError}</InlineMessage>
        )}
        <p>
          <Trans
            i18nKey="serverTransferDrawer.currentSparkOwnershipAccessAndSchedulesStayWithTheServer"
            t={t}
            components={{
              resourceLink: (
                <ResourceLink href={links.spark(server.spark_id)}>
                  {server.spark_name}
                </ResourceLink>
              ),
            }}
          />
        </p>
        {status !== undefined &&
          ![RuntimeState.Stopped, RuntimeState.NotInstalled].some(
            (value) => value === status.state,
          ) && (
            <p className="field-help">
              {t("serverTransferDrawer.stopTheServerBeforeBeginningATransfer")}
            </p>
          )}
        <FormField label={t("serverTransferDrawer.destinationSpark")}>
          <Select
            id="transfer-target"
            value={targetID}
            disabled={busy || active}
            onValueChange={(selectedValue) => selectTarget(selectedValue)}
          >
            <option value="">{t("serverTransferDrawer.selectASpark")}</option>
            {choices.map((spark) => (
              <option key={spark.id} value={spark.id}>
                {spark.name}
                {spark.connected
                  ? ""
                  : ` (${t("sparkConnectionState.disconnected")})`}
              </option>
            ))}
          </Select>
        </FormField>
        {target && (
          <>
            <p className="field-help">
              <Trans
                i18nKey="serverTransferDrawer.gameAddressAfterTransferExistingGameClientsMayNeedThe"
                t={t}
                components={{
                  targetGameAddress: (
                    <>{target.game_address || t("common.notConfigured")}</>
                  ),
                }}
              />
            </p>
            {ports.map((port, index) => (
              <Fieldset
                key={`${port.container_port}-${port.protocol}-${index}`}
                disabled={busy || active}
              >
                <legend>
                  {i18n.t("serverTransferDrawer.containerPort")}
                  {port.container_port}/{port.protocol}
                </legend>
                <FormField
                  label={i18n.t("serverTransferDrawer.destinationInterface")}
                >
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
                    <option value="">
                      {i18n.t("serverTransferDrawer.selectAnInterface")}
                    </option>
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
                  label={i18n.t("serverTransferDrawer.destinationHostPort")}
                  description={
                    <>
                      <Trans
                        i18nKey="serverTransferDrawer.allowedRanges"
                        t={i18n.t}
                        components={{
                          formatDisplayValueTargetPort: (
                            <>
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
                          ),
                        }}
                      />
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
      </div>
    </Drawer>
  );
}
