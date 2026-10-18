import { FormField } from "../components/forms/FormField";
import { TextInput } from "../components/forms/TextInput";
import { Textarea } from "../components/forms/Textarea";
import { InlineMessage } from "../components/status/InlineMessage";
import { Button } from "../components/forms/Button";
import { Select } from "../components/forms/Select";
import { useEffect, useRef, useState } from "react";
import { applyServerEmber, saveServer } from "../api/servers";
import type { Server, ServerConfiguration } from "../api/servers";
import type { Spark } from "../api/sparks";
import type { Ember } from "../api/embers";
import { Drawer } from "../components/layout/Drawer";
import { Wizard } from "../components/wizard/Wizard";
import { ServerResourcesFields } from "../components/servers/ServerResourcesFields";
import { ServerReview } from "../components/servers/ServerReview";
import { defaultPorts, defaultVariables } from "../utils/serverDefaults";
import {
  getBackupPolicy,
  saveBackupPolicy,
  type BackupPolicy,
} from "../api/backups";
import { BackupPolicyFields } from "../components/servers/BackupPolicyFields";
import { ConfirmDialog } from "../components/layout/ConfirmDialog";

interface ServerDialogProps {
  s3Configured: boolean;
  runtimeRestricted?: boolean;
  applyRevision?: boolean;
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
  server,
  sparks,
  embers,
  servers,
  onClose,
  onSaved,
}: ServerDialogProps) {
  const [value, setValue] = useState<ServerConfiguration>(() => {
    if (server) return server;
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
    if (!server || applyRevision) return;
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
  }, [server, applyRevision]);
  const submitting = useRef(false);
  const createdServer = useRef<Server | null>(null);
  const spark = sparks.find((item) => item.id === value.spark_id);
  const ember = embers.find((item) => item.id === value.ember_id);
  async function finish() {
    if (submitting.current) return;
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
        applyRevision && server
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
        applyRevision
          ? "Apply Ember revision"
          : server
            ? "Save changes"
            : "Create server"
      }
      onFinish={async () => {
        if (policyDirty) setConfirmPolicy(true);
        else await finish();
      }}
      steps={[
        {
          id: "basics",
          title: "Basics",
          validate: () =>
            applyRevision && value.ember_id === server?.ember_id
              ? "Select a different imported Ember revision."
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
                  onChange={(event) => {
                    const selected = sparks.find(
                      (item) => item.id === event.target.value,
                    );
                    setValue({
                      ...value,
                      spark_id: event.target.value,
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
                      Current revision: {server?.ember_name} ·{" "}
                      {server?.ember_id}. Select an imported revision
                      explicitly; names do not imply compatibility. Review
                      variables, images, required ports, startup, and ownership
                      before applying.
                    </>
                  )
                }
              >
                <Select
                  id="server-ember"
                  required
                  disabled={server !== null && !applyRevision}
                  value={value.ember_id}
                  onChange={(event) => {
                    const selected = embers.find(
                      (item) => item.id === event.target.value,
                    );
                    setValue({
                      ...value,
                      ember_id: event.target.value,
                      image:
                        applyRevision &&
                        selected &&
                        Object.values(selected.images).includes(value.image)
                          ? value.image
                          : (Object.values(selected?.images ?? {})[0] ?? ""),
                      variables: applyRevision
                        ? Object.fromEntries(
                            (selected?.variables ?? []).map((variable) => [
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
                  onChange={(event) =>
                    setValue({ ...value, image: event.target.value })
                  }
                >
                  {Object.entries(ember?.images ?? {}).map(([label, image]) => (
                    <option key={label} value={image}>
                      {label}
                    </option>
                  ))}
                </Select>
              </FormField>
              {(ember?.variables ?? []).length > 0 && <h4>Game settings</h4>}
              {(ember?.variables ?? []).map((variable) => (
                <div key={variable.name}>
                  <FormField label={<>{variable.name}</>}>
                    <TextInput
                      id={`variable-${variable.name}`}
                      disabled={runtimeRestricted}
                      required={variable.required}
                      readOnly={!variable.editable}
                      maxLength={8192}
                      value={value.variables[variable.name] ?? variable.default}
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
            <ServerResourcesFields
              disabled={runtimeRestricted}
              value={value}
              spark={spark}
              onChange={setValue}
            />
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
                  <BackupPolicyFields
                    value={backupPolicy}
                    s3Available={s3Available}
                    onChange={(next) => {
                      setBackupPolicy(next);
                      setPolicyDirty(true);
                    }}
                  />
                ) : (
                  <p role="status">Loading backup policy…</p>
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
                    Revision: {server?.ember_id} → {value.ember_id}
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
                    {Object.keys(server?.variables ?? {})
                      .filter(
                        (name) =>
                          !(ember?.variables ?? []).some(
                            (variable) => variable.name === name,
                          ),
                      )
                      .join(", ") || "None"}
                    . Shared editable settings are retained; new and locked
                    settings use the selected revision’s defaults.
                  </p>
                </>
              )}
              <ServerReview
                value={value}
                sparkName={spark?.name ?? ""}
                emberName={ember?.name ?? ""}
                notice={
                  applyRevision
                    ? "Applying changes the pinned Ember and saved configuration. Game files and backups are kept. No installer runs and the server stays stopped. Use Reinstall if the revision requires new game files. Take a backup first; revision changes do not roll back game files."
                    : server
                      ? "Saving keeps the server in its current state, including when unavailable. Game configuration changes apply on the next restart. Reducing resources or removing reserved host ports requires confirming the server is stopped."
                      : undefined
                }
              />
            </>
          ),
        },
      ]}
    >
      {(content, actions) => (
        <Drawer
          title={
            applyRevision
              ? "Apply Ember revision"
              : server
                ? "Edit server"
                : "Create server"
          }
          description="Choose where it runs and what it hosts."
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
              title="Save server and backup settings?"
              description="Retention runs in the background and permanently deletes expired copies independently from local and S3 storage. Zero disables a limit."
              confirmLabel="Save changes"
              busy={busy}
              error={error}
              onClose={() => setConfirmPolicy(false)}
              onConfirm={() => {
                void finish();
              }}
            />
          )}
        </Drawer>
      )}
    </Wizard>
  );
}
