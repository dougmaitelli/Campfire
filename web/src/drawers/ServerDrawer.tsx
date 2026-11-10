import { BackupStorage, VariableType } from "../contracts/domains";
import { ResourceLimits } from "../config/policies";
import i18n from "../i18n";
import { BackupRetentionFields } from "../components/servers/BackupRetentionFields";
import { useTranslation, Trans } from "react-i18next";
import { ResourceLink } from "../components/navigation/ResourceLink";
import { useResourceLinks } from "../routing/useResourceLinks";
import { Fieldset } from "../components/forms/Fieldset";
import { FormField } from "../components/forms/FormField";
import { EditableList } from "../components/forms/EditableList";
import { ServerVariableField } from "../components/servers/ServerVariableField";
import "./server-drawer.css";
import { formatDisplayValue } from "../utils/formatDisplayValue";
import { TextInput } from "../components/forms/TextInput";
import { CheckboxField } from "../components/forms/CheckboxField";
import { ToggleSwitch } from "../components/forms/ToggleSwitch";
import { Textarea } from "../components/forms/Textarea";
import { InlineMessage } from "../components/status/InlineMessage";
import { Button } from "../components/forms/Button";
import { Select } from "../components/forms/Select";
import { UserSelect } from "../components/forms/UserSelect";
import { formatUserLabel } from "../utils/formatUserLabel";
import { isEnabledSpark } from "../utils/sparkDestinations";
import { useEffect, useRef, useState } from "react";
import { applyServerEmber, saveServer } from "../api/servers";
import { createServerCopy, type CopyTemplate } from "../api/copies";
import { canPerformPlatform, canPerformServer } from "../permissions";
import type { Server, ServerConfiguration, ServerPort } from "../api/servers";
import type { Spark } from "../api/sparks";
import type { Ember } from "../api/embers";
import { Drawer } from "../components/layout/Drawer";
import { Wizard } from "../components/wizard/Wizard";
import { ServerReview } from "../components/servers/ServerReview";
import {
  copyPorts,
  defaultPorts,
  defaultVariables,
} from "../utils/serverDefaults";
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
import {
  cancelBackupImport,
  publishBackupImport,
  type BackupImport,
} from "../api/backupImports";
import { serverAction, ServerAction } from "../api/runtime";
import { ProgressBar } from "../components/status/ProgressBar";
import { ServerIntegrationsStep } from "../components/servers/ServerIntegrationsStep";
import { useServerIntegrations } from "../hooks/useServerIntegrations";
import {
  serverConfigurationChanges,
  type ConfigurationChange,
} from "../utils/serverConfigurationChanges";
import { ChangeIndicator } from "../components/status/ChangeIndicator";

interface ServerDrawerProps {
  copy?: CopyTemplate | undefined;
  backup?: BackupImport | undefined;
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

export function ServerDrawer({
  copy,
  backup,
  s3Configured,
  runtimeRestricted = false,
  applyRevision = false,
  accessOnly = false,
  server,
  sparks,
  embers,
  servers,
  onClose: closeDrawer,
  onSaved,
}: ServerDrawerProps) {
  const { t } = useTranslation();
  const links = useResourceLinks();
  const session = useAuthentication().session;
  const admin = canPerformPlatform(session, "assignServerOwner");
  const canShare =
    !applyRevision && (!server || canPerformServer(server, "manageAccess"));
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
  const serverID = server?.id;
  const canConfigureIntegrations =
    !!server &&
    !copy &&
    !applyRevision &&
    !accessOnly &&
    canPerformServer(server, "configure");
  const integrations = useServerIntegrations(server, canConfigureIntegrations);
  useEffect(() => {
    if (!canShare) return;
    const controller = new AbortController();
    void Promise.all([
      serverID
        ? readServerAccess(serverID, controller.signal)
        : serverAccessOptions(controller.signal),
      admin ? listUsers(controller.signal) : Promise.resolve([]),
    ])
      .then(([next, accounts]) => {
        if (controller.signal.aborted) return;
        setAccess(next);
        setGrants(next.grants);
        setOwnerID(next.owner?.id ?? (serverID ? 0 : (session?.user_id ?? 0)));
        setUsers(accounts);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setAccessError(
            cause instanceof Error
              ? cause.message
              : i18n.t("serverDrawer.unableToLoadServerAccess"),
          );
      });
    return () => controller.abort();
  }, [canShare, admin, serverID, session?.user_id, accessAttempt]);
  const [value, setValue] = useState<ServerConfiguration>(() => {
    if (copy) {
      const spark =
        sparks.find(
          (item) => item.id === copy.server.spark_id && !item.disabled,
        ) ?? sparks.find((item) => !item.disabled);
      const base = Array.from(copy.server.name);
      let name = "";
      for (let number = 1; ; number++) {
        const suffix = number === 1 ? " (copy)" : ` (copy ${number})`;
        name =
          base.slice(0, ResourceLimits.NameLength - suffix.length).join("") +
          suffix;
        if (
          !servers.some(
            (item) => item.name.toLowerCase() === name.toLowerCase(),
          )
        )
          break;
      }
      return {
        ...copy.server,
        name,
        spark_id: spark?.id ?? "",
        ports: copyPorts(spark, copy.server.ports, servers),
      };
    }
    if (backup) {
      const spark = sparks.find((item) => !item.disabled);
      return {
        ...backup.configuration,
        ember_id: backup.ember.id,
        spark_id: spark?.id ?? "",
        ports: defaultPorts(spark, backup.ember, servers),
      };
    }
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
      name: ember?.name.slice(0, ResourceLimits.NameLength) ?? "",
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
  const importedBackupName = useRef<string | null>(null);
  function onClose() {
    if (backup) void cancelBackupImport(backup.token);
    closeDrawer();
  }
  const [error, setError] = useState<string | null>(null);
  const [copyFiles, setCopyFiles] = useState(false);
  const [backupPolicy, setBackupPolicy] = useState<BackupPolicy | null>(() =>
    copy
      ? copy.policy
      : server
        ? null
        : {
            storage: BackupStorage.Local,
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
    if (!serverID || applyRevision || accessOnly) return;
    const controller = new AbortController();
    void getBackupPolicy(serverID, controller.signal)
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
              : i18n.t("serverDrawer.unableToLoadBackupPolicy"),
          );
      });
    return () => controller.abort();
  }, [serverID, applyRevision, accessOnly]);
  const submitting = useRef(false);
  const [createdServer, setCreatedServer] = useState<Server | null>(null);
  const sparkChoices = sparks.filter(
    (item) => item.id === server?.spark_id || isEnabledSpark(item),
  );
  const spark = sparkChoices.find((item) => item.id === value.spark_id);
  const ember = embers.find((item) => item.id === value.ember_id);
  const visibleVariables = (ember?.variables ?? []).filter(
    (variable) => !variable.hidden,
  );
  const changes =
    applyRevision && server
      ? serverConfigurationChanges(server, value)
      : undefined;
  const removedVariables = Object.entries(changes?.variables ?? {})
    .filter(([, change]) => change === "removed")
    .map(([name]) => name);
  const variablesChanged = Object.keys(changes?.variables ?? {}).length > 0;
  const anyChanges =
    variablesChanged || Object.keys(changes?.fields ?? {}).length > 0;
  function portChange(
    index: number,
    keys: (keyof ServerPort)[],
  ): ConfigurationChange | undefined {
    if (!changes?.fields.ports || !server) return undefined;
    const before = server.ports[index];
    const after = value.ports[index];
    if (!before) return "added";
    return keys.some((key) => before[key] !== after?.[key])
      ? "changed"
      : undefined;
  }
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
    const integrationError = integrations.validate();
    if (integrationError) {
      setError(integrationError);
      return;
    }
    if (canShare && accessDirty && (!access || accessError)) {
      setError(accessError ?? i18n.t("serverDrawer.waitForServerAccessToLoad"));
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
        ember_id: createdServer?.ember_id ?? value.ember_id,
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
      const saved = copy
        ? (createdServer ??
          (await createServerCopy(
            copy.server.id,
            input,
            copyFiles,
            policyDirty ? backupPolicy : null,
            admin ? ownerID : undefined,
          )))
        : accessOnly && server
          ? server
          : applyRevision && server
            ? await applyServerEmber(server.id, server.ember_id, input)
            : await saveServer(
                input,
                server?.id ?? createdServer?.id,
                createdServer ? undefined : backup?.token,
              );
      if (!server) {
        setCreatedServer(saved);
      }
      if (!copy && policyDirty && backupPolicy) {
        try {
          await saveBackupPolicy(saved.id, backupPolicy);
        } catch (cause) {
          onSaved(saved);
          throw new Error(
            i18n.t(
              "serverDrawer.serverSavedButBackupSettingsFailedRetrySavingToApply",
              {
                causeInstanceofError:
                  cause instanceof Error ? cause.message : "",
              },
            ),
            { cause },
          );
        }
      }
      if (integrations.dirty) {
        try {
          await integrations.save();
        } catch (cause) {
          onSaved(saved);
          throw new Error(
            t("serverIntegrations.saveFailed", {
              error:
                cause instanceof Error
                  ? cause.message
                  : t("integrations.failed"),
            }),
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
          if (!copy && admin && ownerID && ownerID !== access?.owner?.id)
            await transferOwner(saved.id, ownerID);
          setAccessDirty(false);
        } catch (cause) {
          onSaved(saved);
          throw new Error(
            i18n.t(
              "serverDrawer.serverSavedButAccessChangesCouldNotBeFullySaved",
              {
                causeInstanceofError:
                  cause instanceof Error ? cause.message : "",
              },
            ),
            { cause },
          );
        }
      }
      if (backup) {
        importedBackupName.current ??= await publishBackupImport(
          saved.id,
          backup.token,
        );
        await serverAction(
          saved.id,
          ServerAction.Restore,
          importedBackupName.current,
        );
      }
      onSaved(saved);
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("serverDrawer.unableToSaveServer"),
      );
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }
  return (
    <Wizard
      className="server-wizard"
      allowAllSteps={server !== null}
      busy={busy}
      error={error}
      finishLabel={
        copy
          ? t("serverCopy.copy")
          : accessOnly
            ? t("serverDrawer.saveAccess")
            : applyRevision
              ? t("serverDrawer.updateEmber")
              : server
                ? t("common.saveChanges")
                : t("serverDrawer.createServer")
      }
      onFinish={async () => {
        if (
          !copy &&
          (policyDirty ||
            (admin &&
              accessDirty &&
              ownerID !==
                (access?.owner?.id ?? (server ? 0 : session?.user_id))))
        )
          setConfirmPolicy(true);
        else await finish();
      }}
      steps={[
        {
          id: "basics",
          title: t("serverDrawer.general"),
          changed: (
            [
              "name",
              "description",
              "image",
              "startup_command",
              "startup_override",
              "game_integration",
            ] as const
          ).some((key) => !!changes?.fields[key]),
          validate: () =>
            copy?.server.ember_update_available
              ? i18n.t("serverCopy.updateRequired")
              : applyRevision && !server?.ember_update_available
                ? i18n.t("serverDrawer.thisServerAlreadyUsesTheCurrentEmber")
                : !value.name.trim() || !spark || !ember || !value.image
                  ? i18n.t("serverDrawer.chooseANameSparkEmberAndImage")
                  : (!server ||
                        applyRevision ||
                        !server.ember_update_available) &&
                      Object.keys(ember.startup_commands ?? {}).length > 0 &&
                      !ember.startup_commands?.[value.startup_command ?? ""]
                    ? i18n.t("serverDrawer.selectAnAvailableStartupCommand")
                    : null,
          content: (
            <>
              {applyRevision && (
                <InlineMessage tone="info">
                  {t("emberUpdate.applyOnRestart")}
                </InlineMessage>
              )}
              {runtimeRestricted && (
                <p className="field-help">
                  {t(
                    "serverDrawer.youCanEditTheNameAndDescriptionDuringAnOperation",
                  )}
                </p>
              )}
              <FormField
                label={t("common.ember")}
                description={
                  applyRevision && (
                    <>
                      <Trans
                        i18nKey="serverDrawer.theLatestDefinitionForWillBeAppliedReviewVariablesImages"
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
                  disabled={
                    server !== null ||
                    backup !== undefined ||
                    copy !== undefined
                  }
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
                          value.name ===
                            ember?.name.slice(0, ResourceLimits.NameLength))
                          ? (selected?.name.slice(
                              0,
                              ResourceLimits.NameLength,
                            ) ?? "")
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
              <FormField
                label={t("serverDrawer.serverName")}
                change={changes?.fields.name}
              >
                <TextInput
                  id="server-name"
                  required
                  maxLength={ResourceLimits.NameLength}
                  value={value.name}
                  onChange={(event) =>
                    setValue({ ...value, name: event.target.value })
                  }
                  placeholder={t("serverDrawer.fridayMinecraft")}
                />
              </FormField>
              <FormField
                label={t("serverDrawer.descriptionOptional")}
                change={changes?.fields.description}
              >
                <Textarea
                  id="server-description"
                  placeholder={t("common.descriptionPlaceholder")}
                  maxLength={ResourceLimits.DescriptionLength}
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
                  disabled={server !== null || createdServer !== null}
                  value={value.spark_id}
                  onValueChange={(selectedValue) => {
                    const selected = sparks.find(
                      (item) => item.id === selectedValue,
                    );
                    setValue({
                      ...value,
                      spark_id: selectedValue,
                      ports: copy
                        ? copyPorts(selected, copy.server.ports, servers)
                        : defaultPorts(selected, ember, servers),
                    });
                  }}
                >
                  {copy && (
                    <option value="" disabled>
                      {t("serverCopy.chooseSpark")}
                    </option>
                  )}
                  {sparkChoices.map((item) => (
                    <option key={item.id} value={item.id}>
                      {item.name}
                      {item.disabled ? " (disabled)" : ""}
                      {copy && !item.connected
                        ? ` (${t("sparkConnectionState.disconnected")})`
                        : ""}
                    </option>
                  ))}
                </Select>
              </FormField>
              {copy && canPerformServer(copy.server, "copyFiles", session) && (
                <div>
                  <CheckboxField
                    aria-describedby="copy-files-help"
                    checked={copyFiles}
                    disabled={createdServer !== null}
                    onChange={(event) => setCopyFiles(event.target.checked)}
                  >
                    {t("serverCopy.copyFiles")}
                  </CheckboxField>
                  <p className="field-help" id="copy-files-help">
                    {t("serverCopy.liveBackup")}
                  </p>
                </div>
              )}
              {Object.keys(ember?.startup_commands ?? {}).length > 0 && (
                <FormField
                  label={t("serverDrawer.startupCommand")}
                  change={changes?.fields.startup_command}
                  description={t(
                    "serverDrawer.chooseTheStartupOptionThisServerWillUseChangesApply",
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
                      {t("serverDrawer.selectAStartupCommand")}
                    </option>
                    {server &&
                      !applyRevision &&
                      value.startup_command &&
                      !ember?.startup_commands?.[value.startup_command] && (
                        <option value={value.startup_command}>
                          {value.startup_command}{" "}
                          {t("serverDrawer.appliedRevision")}
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
                label={t("serverDrawer.customStartupCommand")}
                change={changes?.fields.startup_override}
                labelAction={{
                  label: t("serverDrawer.resetStartupCommand"),
                  disabled:
                    runtimeRestricted ||
                    (!!server?.ember_update_available && !applyRevision),
                  onClick: () =>
                    setValue({ ...value, startup_override: undefined }),
                }}
                description={t("serverDrawer.customStartupCommandHelp")}
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
              <FormField
                label={t("serverDrawer.gameIntegration")}
                change={changes?.fields.game_integration}
              >
                <ToggleSwitch
                  aria-label={t("serverDrawer.gameIntegration")}
                  checked={value.game_integration ?? false}
                  disabled={runtimeRestricted}
                  onChange={(event) =>
                    setValue({
                      ...value,
                      game_integration: event.target.checked,
                    })
                  }
                />
              </FormField>
              <p className="field-help">
                {t("serverDrawer.gameIntegrationHelp")}
              </p>
              <FormField
                label={t("serverDrawer.image")}
                change={changes?.fields.image}
              >
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
            </>
          ),
        },
        ...(visibleVariables.length > 0 || removedVariables.length > 0
          ? [
              {
                id: "variables",
                title: t("serverDrawer.variables"),
                changed: variablesChanged,
                content: (
                  <div className="server-variable-fields">
                    {visibleVariables.map((variable) => (
                      <ServerVariableField
                        key={variable.name}
                        variable={variable}
                        change={changes?.variables[variable.name]}
                        disabled={runtimeRestricted}
                        value={
                          value.variables[variable.name] ?? variable.default
                        }
                        onChange={(next) =>
                          setValue({
                            ...value,
                            variables: {
                              ...value.variables,
                              [variable.name]: next,
                            },
                          })
                        }
                      />
                    ))}
                    {removedVariables.length > 0 && (
                      <div className="server-removed-variables">
                        <h4>{t("changes.removedVariables")}</h4>
                        {removedVariables.map((name) => (
                          <div key={name}>
                            {name}
                            <ChangeIndicator change="removed" />
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                ),
              },
            ]
          : []),
        {
          id: "resources",
          title: t("serverDrawer.resourcesPorts"),
          changed: (
            ["cpu_limit", "memory_mb", "disk_mb", "ports"] as const
          ).some((key) => !!changes?.fields[key]),
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
                  "serverDrawer.mapEveryRequiredEmberContainerPortAndProtocol",
                )
              : null,
          content: (
            <Fieldset disabled={runtimeRestricted}>
              <p className="field-help">
                <Trans
                  i18nKey="serverDrawer.reserveCapacityOnSetTheResourcesAllocatedToThisServer"
                  t={t}
                  components={{
                    sparkNameThe: (
                      <>{spark?.name ?? t("serverDrawer.theSelectedSpark")}</>
                    ),
                  }}
                />
              </p>
              <FormField
                label={t("serverDrawer.cpuCores")}
                change={changes?.fields.cpu_limit}
              >
                <TextInput
                  id="server-cpu"
                  type="number"
                  min={ResourceLimits.ServerMinCPU}
                  max={ResourceLimits.MaxCPU}
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
                  <FormField
                    label={t("serverDrawer.memoryMiB")}
                    change={changes?.fields.memory_mb}
                  >
                    <TextInput
                      id="server-memory"
                      type="number"
                      min={ResourceLimits.MinMemoryMiB}
                      max={ResourceLimits.MaxMemoryMiB}
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
                    label={t("serverDrawer.diskMiB")}
                    change={changes?.fields.disk_mb}
                    description={t(
                      "serverDrawer.use0ForUnlimitedHardLimitsRequireDiskQuotaSupport",
                    )}
                  >
                    <TextInput
                      id="server-disk"
                      type="number"
                      min={0}
                      max={ResourceLimits.MaxDiskMiB}
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
              <h4 className="server-drawer-heading">
                {t("serverDrawer.portMappings")}
                {changes?.fields.ports && <ChangeIndicator change="changed" />}
              </h4>
              <p className="field-help">
                {t(
                  "serverDrawer.hostPortsMustBelongToTheSparkSAllocationRanges",
                )}
              </p>
              <EditableList
                items={value.ports}
                maxItems={32}
                addDisabled={allocations.length === 0}
                rowLabel={(index) =>
                  `${t("serverDrawer.mapping")} ${index + 1}`
                }
                removeLabel={(index) =>
                  t("serverDrawer.removeMapping", { index1: index + 1 })
                }
                addLabel={t("serverDrawer.addMapping")}
                onRemove={(index) =>
                  setValue({
                    ...value,
                    ports: value.ports.filter((_, i) => i !== index),
                  })
                }
                onAdd={() => {
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
                {(port, index) => {
                  const selected = allocations.findIndex(
                    (allocation) =>
                      allocation.bind_ip === port.bind_ip &&
                      allocation.protocol === port.protocol &&
                      port.host_port >= allocation.start &&
                      port.host_port <= allocation.end,
                  );
                  return (
                    <>
                      <FormField
                        label={i18n.t("serverDrawer.hostPort")}
                        change={portChange(index, [
                          "bind_ip",
                          "host_port",
                          "protocol",
                        ])}
                      >
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
                            {i18n.t("serverDrawer.selectAllocation")}
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
                      <FormField
                        label={i18n.t("serverDrawer.containerPort")}
                        change={portChange(index, ["container_port"])}
                      >
                        <TextInput
                          id={`mapping-container-${index}`}
                          type="number"
                          min={1}
                          max={ResourceLimits.MaxPort}
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
                    </>
                  );
                }}
              </EditableList>
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
                    <FormField label={t("serverDrawer.backgroundS3Sync")}>
                      <ToggleSwitch
                        id="backup-storage"
                        aria-label={t("serverDrawer.backgroundS3Sync")}
                        checked={backupPolicy.storage === BackupStorage.S3}
                        disabled={
                          !s3Available &&
                          backupPolicy.storage !== BackupStorage.S3
                        }
                        onChange={(event) =>
                          changeBackupPolicy({
                            ...backupPolicy,
                            storage: event.target.checked
                              ? BackupStorage.S3
                              : BackupStorage.Local,
                          })
                        }
                      />
                    </FormField>
                    {!s3Available && (
                      <p className="field-help">
                        {t(
                          "serverDrawer.configureS3OnCampfireBeforeEnablingSync",
                        )}
                      </p>
                    )}
                    <BackupRetentionFields
                      mode="full"
                      value={backupPolicy}
                      onChange={(limits) =>
                        changeBackupPolicy({ ...backupPolicy, ...limits })
                      }
                    />
                    <BackupRetentionFields
                      mode="savedata"
                      value={
                        backupPolicy.savedata ?? {
                          local_keep_last: 0,
                          local_max_age_days: 0,
                          remote_keep_last: 0,
                          remote_max_age_days: 0,
                        }
                      }
                      onChange={(limits) =>
                        changeBackupPolicy({
                          ...backupPolicy,
                          savedata: limits,
                        })
                      }
                    />
                    <p className="field-help">
                      {t(
                        "serverDrawer.zeroDisablesALimitLocalAndS3CopiesExpireIndependently",
                      )}
                    </p>
                  </>
                ) : (
                  <p role="status">{t("serverDrawer.loadingBackupPolicy")}</p>
                ),
              },
            ]
          : []),
        {
          id: "integrations",
          title: t("integrations.title"),
          validate: integrations.validate,
          content: server && (
            <ServerIntegrationsStep
              server={server}
              state={integrations}
              busy={busy}
            />
          ),
        },
        ...(canShare
          ? [
              {
                id: "access",
                title: t("serverDrawer.access"),
                validate: () =>
                  accessError ??
                  (!access
                    ? i18n.t("serverDrawer.waitForServerAccessToLoad")
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
                      {t("serverDrawer.retry")}
                    </Button>
                  </>
                ) : !access ? (
                  <p role="status">{t("serverDrawer.loadingAccess")}</p>
                ) : (
                  <>
                    {(admin || server) && (
                      <h4 className="server-drawer-heading">
                        {t("serverDrawer.owner")}
                      </h4>
                    )}
                    {admin ? (
                      <FormField
                        label={t("serverDrawer.owner")}
                        description={
                          server
                            ? t(
                                "serverDrawer.changingTheOwnerRemovesThePreviousOwnerSImplicitAccess",
                              )
                            : t("serverDrawer.chooseWhoWillOwnThisServer")
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
                              t("serverDrawer.administratorManaged"))}
                        </ResourceLink>
                      </p>
                    ) : null}
                    <h4 className="server-drawer-heading">
                      {t("serverDrawer.accessGrants")}
                    </h4>
                    <AccessGrantFields
                      users={access.subject_users.filter(
                        (user) => user.id !== ownerID,
                      )}
                      grants={grants}
                      subjectRoles={access.subject_roles}
                      accessRoles={access.roles}
                      accessRoleLabel={t("common.serverAccessRole")}
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
          changed: anyChanges,
          validate: integrations.validate,
          content: (
            <>
              {applyRevision && (
                <InlineMessage tone="info">
                  {t("emberUpdate.applyOnRestart")}
                </InlineMessage>
              )}
              {applyRevision && (
                <>
                  <p>
                    <Trans
                      i18nKey="serverDrawer.revision"
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
                      i18nKey="serverDrawer.startupWorkingDirectoryRuntimeUser"
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
                      i18nKey="serverDrawer.removedSettingsSharedEditableSettingsAreRetainedNewAndLocked"
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
                  changes={changes}
                  variableTypes={Object.fromEntries(
                    (ember?.variables ?? []).map((variable) => [
                      variable.name,
                      variable.validation?.type ?? VariableType.String,
                    ]),
                  )}
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
                    copy && copyFiles
                      ? t("serverCopy.reviewFiles")
                      : backup
                        ? t("backupImports.restoreNotice")
                        : applyRevision
                          ? t(
                              "serverDrawer.applyingUpdatesTheServerSEmberRevisionAndSavedConfiguration",
                            )
                          : server
                            ? t(
                                "serverDrawer.savingKeepsTheServerInItsCurrentStateIncludingWhen",
                              )
                            : undefined
                  }
                />
              )}
              {canShare && access && (
                <>
                  <h4 className="server-drawer-heading">
                    {t("serverDrawer.access")}
                  </h4>
                  <p>
                    <Trans
                      i18nKey="serverDrawer.owner2"
                      t={t}
                      components={{
                        resourceLink: (
                          <ResourceLink
                            href={owner ? links.user(owner.id) : undefined}
                          >
                            {owner
                              ? formatUserLabel(owner)
                              : (session?.username ??
                                t("serverDrawer.administratorManaged"))}
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
                      {t("serverDrawer.noAccessGrantsAssigned")}
                    </p>
                  )}
                </>
              )}
            </>
          ),
        },
      ].filter(
        (step) =>
          (step.id !== "integrations" || canConfigureIntegrations) &&
          (!accessOnly || step.id === "access" || step.id === "review"),
      )}
    >
      {(content, actions) => (
        <Drawer
          title={
            copy
              ? i18n.t("serverCopy.copy")
              : accessOnly
                ? i18n.t("serverDrawer.editServerAccess")
                : applyRevision
                  ? i18n.t("serverDrawer.updateEmber")
                  : server
                    ? i18n.t("serverDrawer.editServer")
                    : i18n.t("serverDrawer.createServer")
          }
          description={
            copy
              ? i18n.t("serverCopy.description", { name: copy.server.name })
              : accessOnly
                ? i18n.t("serverDrawer.manageOwnershipAndCollaborators")
                : i18n.t("serverDrawer.chooseWhereItRunsWhatItHostsAndWhoHas")
          }
          busy={busy}
          onClose={onClose}
          footerLeft={
            <Button type="button" disabled={busy} onClick={onClose}>
              {i18n.t("common.cancel")}
            </Button>
          }
          footerRight={
            copy && createdServer ? (
              <Button disabled={busy} onClick={() => void finish()}>
                {t("common.tryAgain")}
              </Button>
            ) : (
              actions
            )
          }
        >
          {backup && busy && (
            <div className="panel-content">
              <p role="status">{t("backupImports.creating")}</p>
              <ProgressBar aria-label={t("backupImports.progress")} />
            </div>
          )}
          {copy && createdServer ? (
            <div className="panel-content">
              {error && <InlineMessage role="alert">{error}</InlineMessage>}
              <p role="status">
                {t("serverCopy.created", { name: createdServer.name })}
              </p>
            </div>
          ) : (
            content
          )}
          {confirmPolicy && (
            <ConfirmDialog
              title={i18n.t("serverDrawer.saveServerChanges")}
              description={
                policyDirty
                  ? i18n.t(
                      "serverDrawer.retentionRunsInTheBackgroundAndPermanentlyDeletesExpiredCopies",
                    )
                  : i18n.t(
                      "serverDrawer.changingOwnershipRemovesThePreviousOwnerSImplicitAccessTo",
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
                  (access?.owner?.id ?? (server ? 0 : session?.user_id)) && (
                  <p>
                    {i18n.t(
                      "serverDrawer.changingOwnershipAlsoRemovesThePreviousOwnerSImplicitAccess",
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
