import i18n from "../i18n";
import { useTranslation, Trans } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { Fieldset } from "../components/forms/Fieldset";
import { FormField } from "../components/forms/FormField";
import { AllocationRow } from "../components/forms/AllocationRow";
import "./server-dialog.css";
import { formatDisplayValue } from "../utils/formatDisplayValue";
import { TextInput } from "../components/forms/TextInput";
import { CheckboxField } from "../components/forms/CheckboxField";
import { Textarea } from "../components/forms/Textarea";
import { InlineMessage } from "../components/status/InlineMessage";
import { Button } from "../components/forms/Button";
import { Select } from "../components/forms/Select";
import { UserSelect } from "../components/forms/UserSelect";
import { formatUserLabel } from "../utils/formatUserLabel";
import { useEffect, useRef, useState } from "react";
import { applyServerEmber, saveServer } from "../api/servers";
import type { Server, ServerConfiguration, ServerPort } from "../api/servers";
import type { Spark } from "../api/sparks";
import type { Ember } from "../api/embers";
import { Drawer } from "../components/layout/Drawer";
import { Wizard } from "../components/wizard/Wizard";
import { ServerReview } from "../components/servers/ServerReview";
import { defaultPorts, defaultVariables } from "../utils/serverDefaults";
import {
  getBackupPolicy,
  saveBackupPolicy,
  type BackupPolicy,
} from "../api/backups";
import { ConfirmDialog } from "../components/layout/ConfirmDialog";
import { useAuthentication } from "../hooks/useAuthentication";
import {
  listUsers,
  readServerAccess,
  serverAccessOptions,
  saveAccessGrants,
  transferOwner,
  type AccessGrant,
  type ServerAccess,
  type User,
} from "../api/access";

import { AccessGrantFields } from "../components/forms/AccessGrantFields";
import { AccessGrantLabel } from "../components/forms/AccessGrantLabel";

interface ServerDialogProps {
  s3Configured: boolean;
  runtimeRestricted?: boolean;
  applyRevision?: boolean;
  accessOnly?: boolean;
  server: Server | null;
  sparks: Spark[];
  embers: Ember[];
  servers: Server[];
  onClose: () => void;
  onSaved: (server: Server) => void;
}

export function ServerDialog({
  s3Configured,
  runtimeRestricted = false,
  applyRevision = false,
  accessOnly = false,
  server,
  sparks,
  embers,
  servers,
  onClose,
  onSaved,
}: ServerDialogProps) {
  const { t } = useTranslation();
  const links = useResourceLinks();
  const session = useAuthentication().session;
  const admin = session?.admin === true;
  const canShare =
    !applyRevision &&
    (!server || admin || server.permissions.includes("server.share"));
  const [access, setAccess] = useState<ServerAccess | null>(null);
  const [accessError, setAccessError] = useState<string | null>(null);
  const [accessAttempt, setAccessAttempt] = useState(0);
  const [users, setUsers] = useState<User[]>([]);
  const [ownerID, setOwnerID] = useState(session?.user_id ?? 0);
  const owner = admin
    ? users.find((user) => user.id === ownerID)
    : access?.owner;
  const [grants, setGrants] = useState<AccessGrant[]>([]);
  const [accessDirty, setAccessDirty] = useState(false);
  const [removedGrantIDs, setRemovedGrantIDs] = useState<number[]>([]);
  useEffect(() => {
    if (!canShare) return;
    const controller = new AbortController();
    void Promise.all([
      server
        ? readServerAccess(server.id, controller.signal)
        : serverAccessOptions(controller.signal),
      admin ? listUsers(controller.signal) : Promise.resolve([]),
    ])
      .then(([next, accounts]) => {
        if (controller.signal.aborted) return;
        setAccess(next);
        setGrants(next.grants);
        setOwnerID(next.owner?.id ?? (server ? 0 : (session?.user_id ?? 0)));
        setUsers(accounts);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setAccessError(
            cause instanceof Error
              ? cause.message
              : i18n.t("serverDialog.unableToLoadServerAccess"),
          );
      });
    return () => controller.abort();
  }, [canShare, admin, server, session?.user_id, accessAttempt]);
  const [value, setValue] = useState<ServerConfiguration>(() => {
    if (server) {
      if (!applyRevision)
        return {
          ...server,
          startup_override:
            server.startup_override === ""
              ? undefined
              : server.startup_override,
        };
      const latest = embers.find((item) => item.id === server.ember_id);
      return {
        ...server,
        startup_command: latest?.startup_commands?.[
          server.startup_command ?? ""
        ]
          ? (server.startup_command ?? "")
          : "",
        image: Object.values(latest?.images ?? {}).includes(server.image)
          ? server.image
          : (Object.values(latest?.images ?? {})[0] ?? ""),
        variables: Object.fromEntries(
          (latest?.variables ?? [])
            .filter((variable) => !variable.hidden)
            .map((variable) => [
              variable.name,
              variable.editable
                ? (server.variables[variable.name] ?? variable.default)
                : variable.default,
            ]),
        ),
      };
    }
    const spark = sparks.find((item) => !item.disabled);
    const ember = embers[0];
    return {
      name: ember?.name.slice(0, 100) ?? "",
      description: "",
      spark_id: spark?.id ?? "",
      ember_id: ember?.id ?? "",
      image: Object.values(ember?.images ?? {})[0] ?? "",
      startup_command:
        Object.keys(ember?.startup_commands ?? {}).length === 1
          ? (Object.keys(ember?.startup_commands ?? {})[0] ?? "")
          : "",
      variables: defaultVariables(ember),
      cpu_limit: 1,
      memory_mb: 1024,
      disk_mb: 10240,
      game_integration: false,
      ports: defaultPorts(spark, ember, servers),
    };
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [backupPolicy, setBackupPolicy] = useState<BackupPolicy | null>(() =>
    server
      ? null
      : {
          storage: "local",
          local_keep_last: 0,
          local_max_age_days: 0,
          remote_keep_last: 0,
          remote_max_age_days: 0,
        },
  );
  const [policyError, setPolicyError] = useState<string | null>(null);
  const [s3Available, setS3Available] = useState(s3Configured);
  const [policyDirty, setPolicyDirty] = useState(false);
  const [confirmPolicy, setConfirmPolicy] = useState(false);
  useEffect(() => {
    if (!server || applyRevision || accessOnly) return;
    const controller = new AbortController();
    void getBackupPolicy(server.id, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) {
          setBackupPolicy(result.policy);
          setS3Available(result.s3_available);
        }
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setPolicyError(
            cause instanceof Error
              ? cause.message
              : i18n.t("serverDialog.unableToLoadBackupPolicy"),
          );
      });
    return () => controller.abort();
  }, [server, applyRevision, accessOnly]);
  const submitting = useRef(false);
  const createdServer = useRef<Server | null>(null);
  const spark = sparks.find((item) => item.id === value.spark_id);
  const ember = embers.find((item) => item.id === value.ember_id);
  function changePort(index: number, changes: Partial<ServerPort>) {
    setValue({
      ...value,
      ports: value.ports.map((port, i) =>
        i === index ? { ...port, ...changes } : port,
      ),
    });
  }
  const allocations = spark?.port_allocations ?? [];
  function changeBackupPolicy(next: BackupPolicy) {
    setBackupPolicy(next);
    setPolicyDirty(true);
  }

  async function finish() {
    if (submitting.current) return;
    if (canShare && accessDirty && (!access || accessError)) {
      setError(accessError ?? i18n.t("serverDialog.waitForServerAccessToLoad"));
      return;
    }
    submitting.current = true;
    setBusy(true);
    setError(null);
    try {
      // Send only writable configuration fields, never status or display metadata.
      const input: ServerConfiguration = {
        name: value.name,
        description: value.description,
        spark_id: value.spark_id,
        ember_id: value.ember_id,
        image: value.image,
        startup_command: value.startup_command ?? "",
        startup_override: value.startup_override ?? "",
        game_integration: value.game_integration ?? false,
        variables: value.variables,
        cpu_limit: value.cpu_limit,
        memory_mb: value.memory_mb,
        disk_mb: value.disk_mb,
        ports: value.ports,
      };
      const saved =
        accessOnly && server
          ? server
          : applyRevision && server
            ? await applyServerEmber(server.id, server.ember_id, input)
            : await saveServer(input, server?.id ?? createdServer.current?.id);
      if (!server) {
        createdServer.current = saved;
      }
      if (policyDirty && backupPolicy) {
        try {
          await saveBackupPolicy(saved.id, backupPolicy);
        } catch (cause) {
          onSaved(saved);
          throw new Error(
            i18n.t(
              "serverDialog.serverSavedButBackupSettingsFailedRetrySavingToApply",
              {
                causeInstanceofError:
                  cause instanceof Error ? cause.message : "",
              },
            ),
            { cause },
          );
        }
      }
      if (accessDirty) {
        try {
          await saveAccessGrants(
            `/api/v1/servers/${encodeURIComponent(saved.id)}/access`,
            grants.filter((grant) => grant.user_id !== ownerID),
            removedGrantIDs,
          );
          if (admin && ownerID && ownerID !== access?.owner?.id)
            await transferOwner(saved.id, ownerID);
          setAccessDirty(false);
        } catch (cause) {
          onSaved(saved);
          throw new Error(
            i18n.t(
              "serverDialog.serverSavedButAccessChangesCouldNotBeFullySaved",
              {
                causeInstanceofError:
                  cause instanceof Error ? cause.message : "",
              },
            ),
            { cause },
          );
        }
      }
      onSaved(saved);
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("serverDialog.unableToSaveServer"),
      );
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }
  return (
    <Wizard
      allowAllSteps={server !== null}
      busy={busy}
      error={error}
      finishLabel={
        accessOnly
          ? t("serverDialog.saveAccess")
          : applyRevision
            ? t("serverDialog.updateEmber")
            : server
              ? t("common.saveChanges")
              : t("serverDialog.createServer")
      }
      onFinish={async () => {
        if (
          policyDirty ||
          (admin &&
            accessDirty &&
            ownerID !== (access?.owner?.id ?? (server ? 0 : session.user_id)))
        )
          setConfirmPolicy(true);
        else await finish();
      }}
      steps={[
        {
          id: "basics",
          title: t("serverDialog.general"),
          validate: () =>
            applyRevision && !server?.ember_update_available
              ? i18n.t("serverDialog.thisServerAlreadyUsesTheCurrentEmber")
              : !value.name.trim() || !spark || !ember || !value.image
                ? i18n.t("serverDialog.chooseANameSparkEmberAndImage")
                : (!server ||
                      applyRevision ||
                      !server.ember_update_available) &&
                    Object.keys(ember.startup_commands ?? {}).length > 0 &&
                    !ember.startup_commands?.[value.startup_command ?? ""]
                  ? i18n.t("serverDialog.selectAnAvailableStartupCommand")
                  : null,
          content: (
            <>
              {runtimeRestricted && (
                <p className="field-help">
                  {t(
                    "serverDialog.youCanEditTheNameAndDescriptionDuringAnOperation",
                  )}
                </p>
              )}
              <FormField
                label={t("common.ember")}
                description={
                  applyRevision && (
                    <>
                      <Trans
                        i18nKey="serverDialog.theLatestDefinitionForWillBeAppliedReviewVariablesImages"
                        t={t}
                        components={{
                          resourceLink: (
                            <ResourceLink
                              href={
                                server
                                  ? links.ember(server.ember_id)
                                  : undefined
                              }
                            >
                              {server?.ember_name}
                            </ResourceLink>
                          ),
                        }}
                      />
                    </>
                  )
                }
              >
                <Select
                  id="server-ember"
                  required
                  disabled={server !== null}
                  value={value.ember_id}
                  onValueChange={(selectedValue) => {
                    const selected = embers.find(
                      (item) => item.id === selectedValue,
                    );
                    setValue({
                      ...value,
                      name:
                        !server &&
                        (!value.name.trim() ||
                          value.name === ember?.name.slice(0, 100))
                          ? (selected?.name.slice(0, 100) ?? "")
                          : value.name,
                      ember_id: selectedValue,
                      startup_override: undefined,
                      startup_command:
                        Object.keys(selected?.startup_commands ?? {}).length ===
                        1
                          ? (Object.keys(selected?.startup_commands ?? {})[0] ??
                            "")
                          : "",
                      image:
                        applyRevision &&
                        selected &&
                        Object.values(selected.images).includes(value.image)
                          ? value.image
                          : (Object.values(selected?.images ?? {})[0] ?? ""),
                      variables: applyRevision
                        ? Object.fromEntries(
                            (selected?.variables ?? [])
                              .filter((variable) => !variable.hidden)
                              .map((variable) => [
                                variable.name,
                                variable.editable
                                  ? (server?.variables[variable.name] ??
                                    variable.default)
                                  : variable.default,
                              ]),
                          )
                        : defaultVariables(selected),
                      ports: applyRevision
                        ? value.ports
                        : defaultPorts(spark, selected, servers),
                    });
                  }}
                >
                  {embers.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name} · {item.id.slice(-6)}
                    </option>
                  ))}
                </Select>
              </FormField>
              <FormField label={t("serverDialog.serverName")}>
                <TextInput
                  id="server-name"
                  required
                  maxLength={100}
                  value={value.name}
                  onChange={(event) =>
                    setValue({ ...value, name: event.target.value })
                  }
                  placeholder={t("serverDialog.fridayMinecraft")}
                />
              </FormField>
              <FormField label={t("serverDialog.descriptionOptional")}>
                <Textarea
                  id="server-description"
                  placeholder={t("common.descriptionPlaceholder")}
                  maxLength={1000}
                  value={value.description}
                  onChange={(event) =>
                    setValue({ ...value, description: event.target.value })
                  }
                />
              </FormField>
              <FormField label={t("common.spark")}>
                <Select
                  id="server-spark"
                  required
                  disabled={server !== null}
                  value={value.spark_id}
                  onValueChange={(selectedValue) => {
                    const selected = sparks.find(
                      (item) => item.id === selectedValue,
                    );
                    setValue({
                      ...value,
                      spark_id: selectedValue,
                      ports: defaultPorts(selected, ember, servers),
                    });
                  }}
                >
                  {sparks
                    .filter(
                      (item) => item.id === server?.spark_id || !item.disabled,
                    )
                    .map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.name}
                        {item.disabled ? " (disabled)" : ""}
                      </option>
                    ))}
                </Select>
              </FormField>
              {Object.keys(ember?.startup_commands ?? {}).length > 0 && (
                <FormField
                  label={t("serverDialog.startupCommand")}
                  description={t(
                    "serverDialog.chooseTheStartupOptionThisServerWillUseChangesApply",
                  )}
                >
                  <Select
                    id="server-startup-command"
                    required
                    disabled={
                      runtimeRestricted ||
                      (!!server?.ember_update_available && !applyRevision)
                    }
                    value={value.startup_command ?? ""}
                    onValueChange={(selectedValue) =>
                      setValue({
                        ...value,
                        startup_command: selectedValue,
                        startup_override: undefined,
                      })
                    }
                  >
                    <option value="">
                      {t("serverDialog.selectAStartupCommand")}
                    </option>
                    {server &&
                      !applyRevision &&
                      value.startup_command &&
                      !ember?.startup_commands?.[value.startup_command] && (
                        <option value={value.startup_command}>
                          {value.startup_command}{" "}
                          {t("serverDialog.appliedRevision")}
                        </option>
                      )}
                    {Object.keys(ember?.startup_commands ?? {})
                      .sort()
                      .map((name) => (
                        <option key={name} value={name}>
                          {name}
                        </option>
                      ))}
                  </Select>
                </FormField>
              )}
              <FormField
                label={t("serverDialog.customStartupCommand")}
                description={t("serverDialog.customStartupCommandHelp")}
              >
                <Textarea
                  id="server-startup-override"
                  rows={4}
                  maxLength={16384}
                  disabled={
                    runtimeRestricted ||
                    (!!server?.ember_update_available && !applyRevision)
                  }
                  value={
                    value.startup_override ??
                    ember?.startup_commands?.[value.startup_command ?? ""] ??
                    ember?.startup ??
                    ""
                  }
                  onChange={(event) =>
                    setValue({ ...value, startup_override: event.target.value })
                  }
                />
              </FormField>
              <Button
                type="button"
                disabled={
                  runtimeRestricted ||
                  (!!server?.ember_update_available && !applyRevision)
                }
                onClick={() =>
                  setValue({ ...value, startup_override: undefined })
                }
              >
                {t("serverDialog.resetStartupCommand")}
              </Button>
              <CheckboxField
                checked={value.game_integration ?? false}
                disabled={runtimeRestricted}
                onChange={(event) =>
                  setValue({ ...value, game_integration: event.target.checked })
                }
              >
                {t("serverDialog.gameIntegration")}
              </CheckboxField>
              <p className="field-help">
                {t("serverDialog.gameIntegrationHelp")}
              </p>
              <FormField label={t("serverDialog.image")}>
                <Select
                  id="server-image"
                  disabled={runtimeRestricted}
                  required
                  value={value.image}
                  onValueChange={(selectedValue) =>
                    setValue({ ...value, image: selectedValue })
                  }
                >
                  {Object.entries(ember?.images ?? {}).map(([label, image]) => (
                    <option key={label} value={image}>
                      {label}
                    </option>
                  ))}
                </Select>
              </FormField>
              {(ember?.variables ?? []).some(
                (variable) => !variable.hidden,
              ) && (
                <h4 className="server-dialog-heading">
                  {t("serverDialog.gameSettings")}
                </h4>
              )}
              <div className="server-variable-fields">
                {(ember?.variables ?? [])
                  .filter((variable) => !variable.hidden)
                  .map((variable) => (
                    <FormField key={variable.name} label={<>{variable.name}</>}>
                      <TextInput
                        id={`variable-${variable.name}`}
                        disabled={runtimeRestricted}
                        required={variable.required}
                        readOnly={!variable.editable}
                        maxLength={8192}
                        value={
                          value.variables[variable.name] ?? variable.default
                        }
                        onChange={(event) =>
                          setValue({
                            ...value,
                            variables: {
                              ...value.variables,
                              [variable.name]: event.target.value,
                            },
                          })
                        }
                      />
                    </FormField>
                  ))}
              </div>
            </>
          ),
        },
        {
          id: "resources",
          title: t("serverDialog.resourcesPorts"),
          validate: () =>
            value.ports.length > 0 &&
            (ember?.required_ports ?? []).some(
              (required) =>
                !value.ports.some(
                  (port) =>
                    port.container_port === required.container &&
                    port.protocol === required.protocol,
                ),
            )
              ? i18n.t(
                  "serverDialog.mapEveryRequiredEmberContainerPortAndProtocol",
                )
              : null,
          content: (
            <Fieldset disabled={runtimeRestricted}>
              <p className="field-help">
                <Trans
                  i18nKey="serverDialog.reserveCapacityOnSetTheResourcesAllocatedToThisServer"
                  t={t}
                  components={{
                    sparkNameThe: (
                      <>{spark?.name ?? t("serverDialog.theSelectedSpark")}</>
                    ),
                  }}
                />
              </p>
              <FormField label={t("serverDialog.cpuCores")}>
                <TextInput
                  id="server-cpu"
                  type="number"
                  min={0.1}
                  max={1024}
                  step="any"
                  required
                  value={Number.isNaN(value.cpu_limit) ? "" : value.cpu_limit}
                  onChange={(event) =>
                    setValue({
                      ...value,
                      cpu_limit: event.target.valueAsNumber,
                    })
                  }
                />
              </FormField>
              <div className="server-form-grid">
                <div>
                  <FormField label={t("serverDialog.memoryMiB")}>
                    <TextInput
                      id="server-memory"
                      type="number"
                      min={64}
                      max={1048576}
                      step={1}
                      required
                      value={
                        Number.isNaN(value.memory_mb) ? "" : value.memory_mb
                      }
                      onChange={(event) =>
                        setValue({
                          ...value,
                          memory_mb: event.target.valueAsNumber,
                        })
                      }
                    />
                  </FormField>
                </div>
                <div>
                  <FormField
                    label={t("serverDialog.diskMiB")}
                    description={t(
                      "serverDialog.use0ForUnlimitedHardLimitsRequireDiskQuotaSupport",
                    )}
                  >
                    <TextInput
                      id="server-disk"
                      type="number"
                      min={0}
                      max={1073741824}
                      step={1}
                      required
                      value={Number.isNaN(value.disk_mb) ? "" : value.disk_mb}
                      onChange={(event) =>
                        setValue({
                          ...value,
                          disk_mb: event.target.valueAsNumber,
                        })
                      }
                    />
                  </FormField>
                </div>
              </div>
              <h4 className="server-dialog-heading">
                {t("serverDialog.portMappings")}
              </h4>
              <p className="field-help">
                {t(
                  "serverDialog.hostPortsMustBelongToTheSparkSAllocationRanges",
                )}
              </p>
              {value.ports.map((port, index) => {
                const selected = allocations.findIndex(
                  (allocation) =>
                    allocation.bind_ip === port.bind_ip &&
                    allocation.protocol === port.protocol &&
                    port.host_port >= allocation.start &&
                    port.host_port <= allocation.end,
                );
                return (
                  <AllocationRow
                    key={index}
                    label={`${t("serverDialog.mapping")} ${index + 1}`}
                    removeLabel={t("serverDialog.removeMapping", {
                      index1: index + 1,
                    })}
                    onRemove={() =>
                      setValue({
                        ...value,
                        ports: value.ports.filter((_, i) => i !== index),
                      })
                    }
                  >
                    <FormField label={i18n.t("serverDialog.hostPort")}>
                      <Select
                        id={`mapping-allocation-${index}`}
                        required
                        value={
                          selected >= 0 ? `${selected}:${port.host_port}` : ""
                        }
                        onValueChange={(selectedValue) => {
                          const [allocationIndex, hostPort] = selectedValue
                            .split(":")
                            .map(Number);
                          if (
                            allocationIndex === undefined ||
                            hostPort === undefined
                          )
                            return;
                          const next = allocations[allocationIndex];
                          if (next)
                            changePort(index, {
                              bind_ip: next.bind_ip,
                              protocol: next.protocol,
                              host_port: hostPort,
                            });
                        }}
                      >
                        <option value="" disabled>
                          {i18n.t("serverDialog.selectAllocation")}
                        </option>
                        {allocations.flatMap((entry, i) =>
                          Array.from(
                            { length: entry.end - entry.start + 1 },
                            (_, offset) => {
                              const hostPort = entry.start + offset;
                              return (
                                <option
                                  key={`${i}:${hostPort}`}
                                  value={`${i}:${hostPort}`}
                                >
                                  {entry.bind_ip} · {hostPort} /{" "}
                                  {entry.protocol.toUpperCase()}
                                </option>
                              );
                            },
                          ),
                        )}
                      </Select>
                    </FormField>
                    <FormField label={i18n.t("serverDialog.containerPort")}>
                      <TextInput
                        id={`mapping-container-${index}`}
                        type="number"
                        min={1}
                        max={65535}
                        step={1}
                        required
                        value={
                          Number.isNaN(port.container_port)
                            ? ""
                            : port.container_port
                        }
                        onChange={(event) =>
                          changePort(index, {
                            container_port: event.target.valueAsNumber,
                          })
                        }
                      />
                    </FormField>
                  </AllocationRow>
                );
              })}
              <Button
                type="button"
                disabled={allocations.length === 0 || value.ports.length >= 32}
                onClick={() => {
                  const first = allocations[0];
                  if (first)
                    setValue({
                      ...value,
                      ports: [
                        ...value.ports,
                        {
                          bind_ip: first.bind_ip,
                          host_port: first.start,
                          container_port: first.start,
                          protocol: first.protocol,
                        },
                      ],
                    });
                }}
              >
                {t("serverDialog.addMapping")}
              </Button>
            </Fieldset>
          ),
        },
        ...(!applyRevision
          ? [
              {
                id: "backups",
                title: t("common.backups"),
                content: policyError ? (
                  <InlineMessage role="alert">{policyError}</InlineMessage>
                ) : backupPolicy ? (
                  <>
                    <FormField label={t("serverDialog.backgroundS3Sync")}>
                      <Select
                        id="backup-storage"
                        value={backupPolicy.storage}
                        onValueChange={(selectedValue) =>
                          changeBackupPolicy({
                            ...backupPolicy,
                            storage: selectedValue as BackupPolicy["storage"],
                          })
                        }
                      >
                        <option value="local">{t("common.disabled")}</option>
                        <option value="s3" disabled={!s3Available}>
                          {t("common.enabled")}
                        </option>
                      </Select>
                    </FormField>
                    {!s3Available && (
                      <p className="field-help">
                        {t(
                          "serverDialog.configureS3OnCampfireBeforeEnablingSync",
                        )}
                      </p>
                    )}
                    {(
                      [
                        [
                          "local_keep_last",
                          t("serverDialog.localCopiesToKeep"),
                          1000,
                        ],
                        [
                          "local_max_age_days",
                          t("serverDialog.localMaximumAgeDays"),
                          36500,
                        ],
                        [
                          "remote_keep_last",
                          t("serverDialog.s3CopiesToKeep"),
                          1000,
                        ],
                        [
                          "remote_max_age_days",
                          t("serverDialog.s3MaximumAgeDays"),
                          36500,
                        ],
                      ] as const
                    ).map(([key, label, max]) => (
                      <div key={key}>
                        <FormField label={<>{label}</>}>
                          <TextInput
                            id={`backup-${key}`}
                            type="number"
                            min={0}
                            max={max}
                            step={1}
                            required
                            value={backupPolicy[key]}
                            onChange={(event) =>
                              changeBackupPolicy({
                                ...backupPolicy,
                                [key]: Number(event.target.value),
                              })
                            }
                          />
                        </FormField>
                      </div>
                    ))}
                    <p className="field-help">
                      {t(
                        "serverDialog.zeroDisablesALimitLocalAndS3CopiesExpireIndependently",
                      )}
                    </p>
                  </>
                ) : (
                  <p role="status">{t("serverDialog.loadingBackupPolicy")}</p>
                ),
              },
            ]
          : []),
        ...(canShare
          ? [
              {
                id: "access",
                title: t("serverDialog.access"),
                validate: () =>
                  accessError ??
                  (!access
                    ? i18n.t("serverDialog.waitForServerAccessToLoad")
                    : null),
                content: accessError ? (
                  <>
                    <InlineMessage>{accessError}</InlineMessage>
                    <Button
                      type="button"
                      onClick={() => {
                        setAccessError(null);
                        setAccessAttempt((attempt) => attempt + 1);
                      }}
                    >
                      {t("serverDialog.retry")}
                    </Button>
                  </>
                ) : !access ? (
                  <p role="status">{t("serverDialog.loadingAccess")}</p>
                ) : (
                  <>
                    {(admin || server) && (
                      <h4 className="server-dialog-heading">
                        {t("serverDialog.owner")}
                      </h4>
                    )}
                    {admin ? (
                      <FormField
                        label={t("serverDialog.owner")}
                        description={
                          server
                            ? t(
                                "serverDialog.changingTheOwnerRemovesThePreviousOwnerSImplicitAccess",
                              )
                            : t("serverDialog.chooseWhoWillOwnThisServer")
                        }
                      >
                        <UserSelect
                          users={users.filter(
                            (user) =>
                              !user.disabled || user.id === access.owner?.id,
                          )}
                          value={ownerID}
                          onValueChange={(selectedID) => {
                            setOwnerID(selectedID);
                            setAccessDirty(true);
                          }}
                        />
                      </FormField>
                    ) : server ? (
                      <p>
                        <ResourceLink
                          href={owner ? links.user(owner.id) : undefined}
                        >
                          {owner
                            ? formatUserLabel(owner)
                            : (session?.username ??
                              t("serverDialog.administratorManaged"))}
                        </ResourceLink>
                      </p>
                    ) : null}
                    <h4 className="server-dialog-heading">
                      {t("serverDialog.accessGrants")}
                    </h4>
                    <AccessGrantFields
                      users={access.subject_users.filter(
                        (user) => user.id !== ownerID,
                      )}
                      grants={grants}
                      subjectRoles={access.subject_roles}
                      accessRoles={access.roles}
                      onChange={(next, removedID) => {
                        setGrants(next);
                        if (removedID)
                          setRemovedGrantIDs([...removedGrantIDs, removedID]);
                        setAccessDirty(true);
                      }}
                    />
                  </>
                ),
              },
            ]
          : []),
        {
          id: "review",
          title: t("common.review"),
          content: (
            <>
              {applyRevision && (
                <>
                  <p>
                    <Trans
                      i18nKey="serverDialog.revision"
                      t={t}
                      components={{
                        serverEmberHash: <>{server?.ember_hash.slice(0, 12)}</>,
                        emberRevisionSlice: (
                          <>{ember?.revision?.slice(0, 12)}</>
                        ),
                      }}
                    />
                  </p>
                  <p>
                    <Trans
                      i18nKey="serverDialog.startupWorkingDirectoryRuntimeUser"
                      t={t}
                      components={{
                        code: (
                          <code>
                            {ember?.startup_commands?.[
                              value.startup_command ?? ""
                            ] ?? ember?.startup}
                          </code>
                        ),
                        br: <br />,
                        emberWorkDir: <>{ember?.work_dir}</>,
                        br2: <br />,
                        emberUser: <>{ember?.user}</>,
                      }}
                    />
                  </p>
                  <p>
                    <Trans
                      i18nKey="serverDialog.removedSettingsSharedEditableSettingsAreRetainedNewAndLocked"
                      t={t}
                      components={{
                        formatDisplayValueObjectKeys: (
                          <>
                            {formatDisplayValue(
                              Object.keys(server?.variables ?? {})
                                .filter(
                                  (name) =>
                                    !(ember?.variables ?? []).some(
                                      (variable) => variable.name === name,
                                    ),
                                )
                                .join(", "),
                            )}
                          </>
                        ),
                      }}
                    />
                  </p>
                </>
              )}
              {!accessOnly && (
                <ServerReview
                  value={value}
                  portDescriptions={Object.fromEntries(
                    (ember?.required_ports ?? []).map((port) => [
                      `${port.container}/${port.protocol}`,
                      port.description ?? "",
                    ]),
                  )}
                  sparkName={
                    <ResourceLink href={links.spark(value.spark_id)}>
                      {spark?.name ?? ""}
                    </ResourceLink>
                  }
                  emberName={
                    <ResourceLink href={links.ember(value.ember_id)}>
                      {ember?.name ?? ""}
                    </ResourceLink>
                  }
                  notice={
                    applyRevision
                      ? t(
                          "serverDialog.applyingUpdatesTheServerSEmberRevisionAndSavedConfiguration",
                        )
                      : server
                        ? t(
                            "serverDialog.savingKeepsTheServerInItsCurrentStateIncludingWhen",
                          )
                        : undefined
                  }
                />
              )}
              {canShare && access && (
                <>
                  <h4 className="server-dialog-heading">
                    {t("serverDialog.access")}
                  </h4>
                  <p>
                    <Trans
                      i18nKey="serverDialog.owner2"
                      t={t}
                      components={{
                        resourceLink: (
                          <ResourceLink
                            href={owner ? links.user(owner.id) : undefined}
                          >
                            {owner
                              ? formatUserLabel(owner)
                              : (session?.username ??
                                t("serverDialog.administratorManaged"))}
                          </ResourceLink>
                        ),
                      }}
                    />
                  </p>
                  {grants.length ? (
                    <ul>
                      {grants.map((grant) => (
                        <li
                          key={
                            grant.role_id === null
                              ? grant.username
                              : `role:${grant.role_id}`
                          }
                        >
                          <AccessGrantLabel grant={grant} />
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="field-help">
                      {t("serverDialog.noAccessGrantsAssigned")}
                    </p>
                  )}
                </>
              )}
            </>
          ),
        },
      ].filter(
        (step) => !accessOnly || step.id === "access" || step.id === "review",
      )}
    >
      {(content, actions) => (
        <Drawer
          title={
            accessOnly
              ? i18n.t("serverDialog.editServerAccess")
              : applyRevision
                ? i18n.t("serverDialog.updateEmber")
                : server
                  ? i18n.t("serverDialog.editServer")
                  : i18n.t("serverDialog.createServer")
          }
          description={
            accessOnly
              ? i18n.t("serverDialog.manageOwnershipAndCollaborators")
              : i18n.t("serverDialog.chooseWhereItRunsWhatItHostsAndWhoHas")
          }
          busy={busy}
          onClose={onClose}
          footerLeft={
            <Button
              type="button"

              disabled={busy}
              onClick={onClose}
            >
              {i18n.t("common.cancel")}
            </Button>
          }
          footerRight={actions}
        >
          {content}
          {confirmPolicy && (
            <ConfirmDialog
              title={i18n.t("serverDialog.saveServerChanges")}
              description={
                policyDirty
                  ? i18n.t(
                      "serverDialog.retentionRunsInTheBackgroundAndPermanentlyDeletesExpiredCopies",
                    )
                  : i18n.t(
                      "serverDialog.changingOwnershipRemovesThePreviousOwnerSImplicitAccessTo",
                    )
              }
              confirmLabel={i18n.t("common.saveChanges")}
              busy={busy}
              error={error}
              onClose={() => setConfirmPolicy(false)}
              onConfirm={() => {
                void finish();
              }}
            >
              {policyDirty &&
                admin &&
                accessDirty &&
                ownerID !==
                  (access?.owner?.id ?? (server ? 0 : session.user_id)) && (
                  <p>
                    {i18n.t(
                      "serverDialog.changingOwnershipAlsoRemovesThePreviousOwnerSImplicitAccess",
                    )}
                  </p>
                )}
            </ConfirmDialog>
          )}
        </Drawer>
      )}
    </Wizard>
  );
}
