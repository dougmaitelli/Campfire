import { useRef, useState } from "react";
import { applyServerEmber, saveServer } from "../api/servers";
import type { Server, ServerConfiguration } from "../api/servers";
import type { Spark } from "../api/sparks";
import type { Ember } from "../api/embers";
import { Drawer } from "../components/layout/Drawer";
import { Wizard } from "../components/wizard/Wizard";
import { ServerResourcesFields } from "../components/servers/ServerResourcesFields";
import { ServerReview } from "../components/servers/ServerReview";
import { defaultPorts, defaultVariables } from "../utils/serverDefaults";

interface ServerDialogProps {
  applyRevision?: boolean;
  server: Server | null;
  sparks: Spark[];
  embers: Ember[];
  servers: Server[];
  onClose: () => void;
  onSaved: (server: Server) => void;
}

export function ServerDialog({
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
  const submitting = useRef(false);
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
      onSaved(
        applyRevision && server
          ? await applyServerEmber(server.id, server.ember_id, input)
          : await saveServer(input, server?.id),
      );
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
      onFinish={finish}
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
              <label htmlFor="server-name">Server name</label>
              <input
                id="server-name"
                required
                maxLength={100}
                value={value.name}
                onChange={(event) =>
                  setValue({ ...value, name: event.target.value })
                }
                placeholder="Friday Minecraft"
              />
              <label htmlFor="server-description">Description (optional)</label>
              <textarea
                id="server-description"
                maxLength={1000}
                rows={3}
                value={value.description}
                onChange={(event) =>
                  setValue({ ...value, description: event.target.value })
                }
              />
              <label htmlFor="server-spark">Spark</label>
              <select
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
              </select>
              <label htmlFor="server-ember">Ember</label>
              {applyRevision && (
                <p className="field-help">
                  Current revision: {server?.ember_name} · {server?.ember_id}.
                  Select an imported revision explicitly; names do not imply
                  compatibility. Review variables, images, required ports,
                  startup, and ownership before applying.
                </p>
              )}
              <select
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
              </select>
              <label htmlFor="server-image">Image</label>
              <select
                id="server-image"
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
              </select>
              {(ember?.variables ?? []).length > 0 && <h4>Game settings</h4>}
              {(ember?.variables ?? []).map((variable) => (
                <div key={variable.name}>
                  <label htmlFor={`variable-${variable.name}`}>
                    {variable.name}
                  </label>
                  <input
                    id={`variable-${variable.name}`}
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
              value={value}
              spark={spark}
              onChange={setValue}
            />
          ),
        },
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
                    ? "Applying changes the pinned Ember and saved configuration. Game files and backups are kept. No installer runs and the server stays stopped. Use Update or Reinstall separately if the revision requires new game files. Take a backup first; revision changes do not roll back game files."
                    : server
                      ? "Saving updates the configuration and reservations. The server stays stopped."
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
            <button
              type="button"
              className="button"
              disabled={busy}
              onClick={onClose}
            >
              Cancel
            </button>
          }
          footerRight={actions}
        >
          {content}
        </Drawer>
      )}
    </Wizard>
  );
}
