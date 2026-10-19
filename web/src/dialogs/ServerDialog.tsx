import { Fieldset } from "../components/forms/Fieldset";
import { FormField } from "../components/forms/FormField";
import "./server-dialog.css";
import { formatDisplayValue } from "../utils/formatDisplayValue";
import { TextInput } from "../components/forms/TextInput";
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
              : "Unable to load server access.",
          );
      });
    return () => controller.abort();
  }, [canShare, admin, server, session?.user_id, accessAttempt]);
  const [value, setValue] = useState<ServerConfiguration>(() => {
    if (server) {
      if (!applyRevision) return server;
      const latest = embers.find((item) => item.id === server.ember_id);
      return {
        ...server,
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
    const spark = sparks.find(
      (item) => !item.disabled && item.port_allocations.length > 0,
    );
    const ember = embers[0];
    return {
      name: "",
      description: "",
      spark_id: spark?.id ?? "",
      ember_id: ember?.id ?? "",
      image: Object.values(ember?.images ?? {})[0] ?? "",
      variables: defaultVariables(ember),
      cpu_limit: 1,
      memory_mb: 1024,
      disk_mb: 10240,
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
              : "Unable to load backup policy.",
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
      setError(accessError ?? "Wait for server access to load.");
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
            `Server saved, but backup settings failed. Retry saving to apply them to the same server. ${cause instanceof Error ? cause.message : ""}`,
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
            `Server saved, but access changes could not be fully saved. Retry to finish saving the same server. ${cause instanceof Error ? cause.message : ""}`,
            { cause },
          );
        }
      }
      onSaved(saved);
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to save server.",
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
          ? "Save access"
          : applyRevision
            ? "Update Ember"
            : server
              ? "Save changes"
              : "Create server"
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
          title: "General",
          validate: () =>
            applyRevision && !server?.ember_update_available
              ? "This server already uses the current Ember."
              : !value.name.trim() || !spark || !ember || !value.image
                ? "Choose a name, Spark, Ember, and image."
                : null,
          content: (
            <>
              {runtimeRestricted && (
                <p className="field-help">
                  You can edit the name and description during an operation.
                  Game settings, resources and ports are locked until it
                  finishes.
                </p>
              )}
              <FormField label="Server name">
                <TextInput
                  id="server-name"
                  required
                  maxLength={100}
                  value={value.name}
                  onChange={(event) =>
                    setValue({ ...value, name: event.target.value })
                  }
                  placeholder="Friday Minecraft"
                />
              </FormField>
              <FormField label="Description (optional)">
                <Textarea
                  id="server-description"
                  maxLength={1000}
                  value={value.description}
                  onChange={(event) =>
                    setValue({ ...value, description: event.target.value })
                  }
                />
              </FormField>
              <FormField label="Spark">
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
                      (item) =>
                        item.id === server?.spark_id ||
                        (!item.disabled && item.port_allocations.length > 0),
                    )
                    .map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.name}
                        {item.disabled ? " (disabled)" : ""}
                      </option>
                    ))}
                </Select>
              </FormField>
              <FormField
                label="Ember"
                description={
                  applyRevision && (
                    <>
                      The latest definition for {server?.ember_name} will be
                      applied. Review variables, images, required ports,
                      startup, and ownership before applying.
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
                      ember_id: selectedValue,
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
              <FormField label="Image">
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
              ) && <h4 className="server-dialog-heading">Game settings</h4>}
              {(ember?.variables ?? [])
                .filter((variable) => !variable.hidden)
                .map((variable) => (
                  <div key={variable.name}>
                    <FormField label={<>{variable.name}</>}>
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
                  </div>
                ))}
            </>
          ),
        },
        {
          id: "resources",
          title: "Resources & ports",
          validate: () =>
            value.ports.length === 0
              ? "Add at least one port mapping."
              : (ember?.required_ports ?? []).some(
                    (required) =>
                      !value.ports.some(
                        (port) =>
                          port.container_port === required.container &&
                          port.protocol === required.protocol,
                      ),
                  )
                ? "Map every required Ember container port and protocol."
                : null,
          content: (
            <Fieldset disabled={runtimeRestricted}>
              <p className="field-help">
                Reserve capacity on {spark?.name ?? "the selected Spark"}. Set
                the resources allocated to this server.
              </p>
              <FormField label="CPU cores">
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
                  <FormField label="Memory (MiB)">
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
                    label="Disk (MiB)"
                    description="Use 0 for unlimited. Hard limits require disk quota support on the Spark."
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
              <h4 className="server-dialog-heading">Port mappings</h4>
              <p className="field-help">
                Host ports must belong to the Spark’s allocation ranges and be
                available. Container ports must match the Ember requirements.
              </p>
              {value.ports.map((port, index) => {
                const selected = allocations.findIndex(
                  (allocation) =>
                    allocation.bind_ip === port.bind_ip &&
                    allocation.protocol === port.protocol &&
                    port.host_port >= allocation.start &&
                    port.host_port <= allocation.end,
                );
                const allocation = allocations[selected];
                return (
                  <Fieldset className="port-mapping" key={index}>
                    <legend>Mapping {index + 1}</legend>
                    <FormField label="Spark allocation">
                      <Select
                        id={`mapping-allocation-${index}`}
                        required
                        value={selected >= 0 ? String(selected) : ""}
                        onValueChange={(selectedValue) => {
                          const next = allocations[Number(selectedValue)];
                          if (next)
                            changePort(index, {
                              bind_ip: next.bind_ip,
                              protocol: next.protocol,
                              host_port: next.start,
                            });
                        }}
                      >
                        <option value="" disabled>
                          Select allocation
                        </option>
                        {allocations.map((entry, i) => (
                          <option key={i} value={i}>
                            {entry.bind_ip} · {entry.start}–{entry.end} /{" "}
                            {entry.protocol.toUpperCase()}
                          </option>
                        ))}
                      </Select>
                    </FormField>
                    <div className="server-form-grid">
                      <div>
                        <FormField label="Host port">
                          <TextInput
                            id={`mapping-host-${index}`}
                            type="number"
                            min={allocation?.start ?? 1}
                            max={allocation?.end ?? 65535}
                            step={1}
                            required
                            value={
                              Number.isNaN(port.host_port) ? "" : port.host_port
                            }
                            onChange={(event) =>
                              changePort(index, {
                                host_port: event.target.valueAsNumber,
                              })
                            }
                          />
                        </FormField>
                      </div>
                      <div>
                        <FormField label="Container port">
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
                      </div>
                    </div>
                    <Button
                      type="button"
                      variant="danger"
                      onClick={() =>
                        setValue({
                          ...value,
                          ports: value.ports.filter((_, i) => i !== index),
                        })
                      }
                      aria-label={`Remove mapping ${index + 1}`}
                    >
                      Remove mapping
                    </Button>
                  </Fieldset>
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
                + Add mapping
              </Button>
            </Fieldset>
          ),
        },
        ...(!applyRevision
          ? [
              {
                id: "backups",
                title: "Backups",
                content: policyError ? (
                  <InlineMessage role="alert">{policyError}</InlineMessage>
                ) : backupPolicy ? (
                  <>
                    <FormField label="Background S3 sync">
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
                        <option value="local">Disabled</option>
                        <option value="s3" disabled={!s3Available}>
                          Enabled
                        </option>
                      </Select>
                    </FormField>
                    {!s3Available && (
                      <p className="field-help">
                        Configure S3 on Campfire before enabling sync.
                      </p>
                    )}
                    {(
                      [
                        ["local_keep_last", "Local copies to keep", 1000],
                        [
                          "local_max_age_days",
                          "Local maximum age (days)",
                          36500,
                        ],
                        ["remote_keep_last", "S3 copies to keep", 1000],
                        ["remote_max_age_days", "S3 maximum age (days)", 36500],
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
                      Zero disables a limit. Local and S3 copies expire
                      independently. A local copy awaiting a required S3 upload
                      is retained until sync succeeds.
                    </p>
                  </>
                ) : (
                  <p role="status">Loading backup policy…</p>
                ),
              },
            ]
          : []),
        ...(canShare
          ? [
              {
                id: "access",
                title: "Access",
                validate: () =>
                  accessError ??
                  (!access ? "Wait for server access to load." : null),
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
                      Retry
                    </Button>
                  </>
                ) : !access ? (
                  <p role="status">Loading access…</p>
                ) : (
                  <>
                    {(admin || server) && (
                      <h4 className="server-dialog-heading">Owner</h4>
                    )}
                    {admin ? (
                      <FormField
                        label="Owner"
                        description={
                          server
                            ? "Changing the owner removes the previous owner’s implicit access when you save."
                            : "Choose who will own this server."
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
                        {owner
                          ? formatUserLabel(owner)
                          : (session?.username ?? "Administrator managed")}
                      </p>
                    ) : null}
                    <h4 className="server-dialog-heading">Access grants</h4>
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
          title: "Review",
          content: (
            <>
              {applyRevision && (
                <>
                  <p>
                    Revision: {server?.ember_hash.slice(0, 12)} →{" "}
                    {ember?.revision?.slice(0, 12)}
                  </p>
                  <p>
                    Startup: <code>{ember?.startup}</code>
                    <br />
                    Working directory: {ember?.work_dir}
                    <br />
                    Runtime user: {ember?.user}
                  </p>
                  <p>
                    Removed settings:{" "}
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
                    . Shared editable settings are retained; new and locked
                    settings use the current definition’s defaults.
                  </p>
                </>
              )}
              {!accessOnly && (
                <ServerReview
                  value={value}
                  sparkName={spark?.name ?? ""}
                  emberName={ember?.name ?? ""}
                  notice={
                    applyRevision
                      ? "Applying updates the server’s Ember revision and saved configuration. Game files and backups are kept. No installer runs and the server stays stopped. Use Reinstall if the revision requires new game files. Take a backup first; revision changes do not roll back game files."
                      : server
                        ? "Saving keeps the server in its current state, including when unavailable. Game configuration changes apply on the next restart. Reducing resources or removing reserved host ports requires confirming the server is stopped."
                        : undefined
                  }
                />
              )}
              {canShare && access && (
                <>
                  <h4 className="server-dialog-heading">Access</h4>
                  <p>
                    Owner:{" "}
                    {owner
                      ? formatUserLabel(owner)
                      : (session?.username ?? "Administrator managed")}
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
                          {grant.role_id === null
                            ? grant.username
                            : `Role: ${grant.role_name}`}{" "}
                          · {grant.access_role}
                        </li>
                      ))}
                    </ul>
                  ) : (
                    <p className="field-help">No access grants assigned.</p>
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
              ? "Edit server access"
              : applyRevision
                ? "Update Ember"
                : server
                  ? "Edit server"
                  : "Create server"
          }
          description={
            accessOnly
              ? "Manage ownership and collaborators."
              : "Choose where it runs, what it hosts, and who has access."
          }
          busy={busy}
          onClose={onClose}
          footerLeft={
            <Button
              type="button"

              disabled={busy}
              onClick={onClose}
            >
              Cancel
            </Button>
          }
          footerRight={actions}
        >
          {content}
          {confirmPolicy && (
            <ConfirmDialog
              title="Save server changes?"
              description={
                policyDirty
                  ? "Retention runs in the background and permanently deletes expired copies independently from local and S3 storage. Zero disables a limit."
                  : "Changing ownership removes the previous owner’s implicit access to this server."
              }
              confirmLabel="Save changes"
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
                    Changing ownership also removes the previous owner’s
                    implicit access to this server.
                  </p>
                )}
            </ConfirmDialog>
          )}
        </Drawer>
      )}
    </Wizard>
  );
}
