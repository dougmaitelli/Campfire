import { FormField } from "../components/forms/FormField";
import { TextInput } from "../components/forms/TextInput";
import { Textarea } from "../components/forms/Textarea";
import { Button } from "../components/forms/Button";
import { Select } from "../components/forms/Select";
import { useEffect, useRef, useState } from "react";
import {
  sparkAccessOptions,
  readSparkAccess,
  saveAccessGrants,
  type AccessGrant,
  type Role,
  type AccessUser,
} from "../api/access";
import { InlineMessage } from "../components/status/InlineMessage";
import { createSpark, updateSpark } from "../api/sparks";
import type { PortAllocation, Spark, SparkSettings } from "../api/sparks";
import { Wizard } from "../components/wizard/Wizard";
import { SparkReview } from "../components/sparks/SparkReview";
import { Drawer } from "../components/layout/Drawer";
import { CheckboxField } from "../components/forms/CheckboxField";
import "./spark-dialog.css";
import { AccessGrantFields } from "../components/forms/AccessGrantFields";

interface SparkDialogProps {
  spark: Spark | null;
  onClose: () => void;
  onSaved: (spark: Spark) => void;
}

export function SparkDialog({ spark, onClose, onSaved }: SparkDialogProps) {
  const submittingRef = useRef(false);
  const [created, setCreated] = useState<Spark | null>(null);
  const [roles, setRoles] = useState<Role[]>([]);
  const [users, setUsers] = useState<AccessUser[]>([]);
  const [grants, setGrants] = useState<AccessGrant[]>([]);
  const [removedIDs, setRemovedIDs] = useState<number[]>([]);
  const [rolesLoading, setRolesLoading] = useState(true);
  const [rolesError, setRolesError] = useState<string | null>(null);
  const [rolesAttempt, setRolesAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    void (
      spark
        ? readSparkAccess(spark.id, controller.signal)
        : sparkAccessOptions(controller.signal)
    )
      .then((access) => {
        if (controller.signal.aborted) return;
        setRoles(access.subject_roles);
        setUsers(access.subject_users);
        setGrants(access.grants);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setRolesError(
            cause instanceof Error
              ? cause.message
              : "Unable to load server creation access.",
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) setRolesLoading(false);
      });
    return () => controller.abort();
  }, [spark, rolesAttempt]);
  const [name, setName] = useState(spark?.name ?? "");
  const [description, setDescription] = useState(spark?.description ?? "");
  const [disabled, setDisabled] = useState(spark?.disabled ?? false);
  const [settings, setSettings] = useState<SparkSettings>(() => ({
    game_address: spark?.game_address ?? "",
    cpu_limit: spark?.cpu_limit ?? 0,
    memory_mb: spark?.memory_mb ?? 0,
    disk_mb: spark?.disk_mb ?? 0,
    port_allocations: spark?.port_allocations ?? [],
  }));
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function changeAllocation(index: number, fields: Partial<PortAllocation>) {
    setSettings({
      ...settings,
      port_allocations: settings.port_allocations.map((entry, i) =>
        i === index ? { ...entry, ...fields } : entry,
      ),
    });
  }

  async function submit() {
    if (submittingRef.current) return;
    if (rolesLoading || rolesError) {
      setError(rolesError ?? "Wait for server creation access to load.");
      return;
    }
    submittingRef.current = true;
    setSubmitting(true);
    setError(null);
    try {
      const existing = spark ?? created;
      const result = existing
        ? await updateSpark(existing.id, { description, disabled, ...settings })
        : await createSpark({
            name: name.trim(),
            description,
            disabled,
            ...settings,
          });
      if (!spark) setCreated(result);
      try {
        await saveAccessGrants(
          `/api/v1/sparks/${encodeURIComponent(result.id)}/access`,
          grants,
          removedIDs,
        );
      } catch (cause) {
        throw new Error(
          `Spark saved, but server creation access could not be saved. Retry to finish saving. ${cause instanceof Error ? cause.message : ""}`,
          { cause },
        );
      }
      onSaved(result);
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to add the Spark.",
      );
    } finally {
      submittingRef.current = false;
      setSubmitting(false);
    }
  }

  return (
    <Wizard
      allowAllSteps={spark !== null}
      busy={submitting}
      error={error}
      finishLabel={spark ? "Save changes" : "Add Spark"}
      onFinish={submit}
      steps={[
        {
          id: "basics",
          title: "General",
          validate: () => (name.trim() === "" ? "Enter a Spark name." : null),
          content: (
            <>
              <FormField
                label="Name"
                description="Run the Spark agent on this host or another machine, then enroll it with Campfire."
              >
                <TextInput
                  id="spark-name"
                  name="name"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  required
                  maxLength={100}
                  placeholder="e.g. Home server"
                  autoComplete="off"
                  readOnly={spark !== null || created !== null}
                />
              </FormField>
              <FormField
                label={
                  <>
                    Description <span className="field-help">(optional)</span>
                  </>
                }
              >
                <Textarea
                  id="spark-description"
                  name="description"
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                  maxLength={1000}
                  placeholder="Notes about this node"
                />
              </FormField>
              <CheckboxField
                id="spark-disabled"
                checked={disabled}
                onChange={(event) => setDisabled(event.target.checked)}
              >
                Disabled
              </CheckboxField>
              <p className="field-help">
                Keep this Spark unavailable for hosting game servers.
              </p>
            </>
          ),
        },
        {
          id: "resources",
          title: "Resources & networking",
          content: (
            <div className="spark-settings">
              <h3>Resource limits</h3>
              <p className="field-help">
                Capacity available for game servers. Set a limit to 0 for no
                configured limit.
              </p>
              <div className="resource-fields">
                <div>
                  <FormField label="CPU cores">
                    <TextInput
                      id="spark-cpu"
                      type="number"
                      min={0}
                      max={1024}
                      step="any"
                      required
                      value={
                        Number.isNaN(settings.cpu_limit)
                          ? ""
                          : settings.cpu_limit
                      }
                      onChange={(event) =>
                        setSettings({
                          ...settings,
                          cpu_limit: event.target.valueAsNumber,
                        })
                      }
                    />
                  </FormField>
                </div>
                <div>
                  <FormField label="Memory (MiB)">
                    <TextInput
                      id="spark-memory"
                      type="number"
                      min={0}
                      max={Number.MAX_SAFE_INTEGER}
                      step={1}
                      required
                      value={
                        Number.isNaN(settings.memory_mb)
                          ? ""
                          : settings.memory_mb
                      }
                      onChange={(event) =>
                        setSettings({
                          ...settings,
                          memory_mb: event.target.valueAsNumber,
                        })
                      }
                    />
                  </FormField>
                </div>
                <div>
                  <FormField label="Disk (MiB)">
                    <TextInput
                      id="spark-disk"
                      type="number"
                      min={0}
                      max={Number.MAX_SAFE_INTEGER}
                      step={1}
                      required
                      value={
                        Number.isNaN(settings.disk_mb) ? "" : settings.disk_mb
                      }
                      onChange={(event) =>
                        setSettings({
                          ...settings,
                          disk_mb: event.target.valueAsNumber,
                        })
                      }
                    />
                  </FormField>
                </div>
              </div>
              <h3>Network</h3>
              <FormField
                label={
                  <>
                    Game address <span className="field-help">(optional)</span>
                  </>
                }
              >
                <TextInput
                  id="spark-address"
                  value={settings.game_address}
                  maxLength={253}
                  placeholder="games.example.com or an IP"
                  onChange={(event) =>
                    setSettings({
                      ...settings,
                      game_address: event.target.value,
                    })
                  }
                  aria-describedby="game-address-help"
                />
              </FormField>
              <p id="game-address-help" className="field-help">
                The address players use to reach game servers. Enter an IP or
                domain without a protocol or port.
              </p>
              <h3>Port allocations</h3>
              <p className="field-help">
                Game ports available on this Spark. Use the same start and end
                for one port; 0.0.0.0 binds all IPv4 interfaces.
              </p>
              {settings.port_allocations.map((allocation, index) => (
                <fieldset className="allocation-range" key={index}>
                  <legend className="visually-hidden">Range {index + 1}</legend>
                  <FormField label="Bind IP" className="allocation-ip">
                    <TextInput
                      id={`allocation-ip-${index}`}
                      required
                      value={allocation.bind_ip}
                      onChange={(event) =>
                        changeAllocation(index, { bind_ip: event.target.value })
                      }
                      placeholder="0.0.0.0"
                    />
                  </FormField>
                  <div className="allocation-ports">
                    <div>
                      <FormField label="Start port">
                        <TextInput
                          id={`allocation-start-${index}`}
                          type="number"
                          min={1}
                          max={65535}
                          step={1}
                          required
                          value={
                            Number.isNaN(allocation.start)
                              ? ""
                              : allocation.start
                          }
                          onChange={(event) =>
                            changeAllocation(index, {
                              start: event.target.valueAsNumber,
                            })
                          }
                        />
                      </FormField>
                    </div>
                    <div>
                      <FormField label="End port">
                        <TextInput
                          id={`allocation-end-${index}`}
                          type="number"
                          min={allocation.start}
                          max={65535}
                          step={1}
                          required
                          value={
                            Number.isNaN(allocation.end) ? "" : allocation.end
                          }
                          onChange={(event) =>
                            changeAllocation(index, {
                              end: event.target.valueAsNumber,
                            })
                          }
                        />
                      </FormField>
                    </div>
                  </div>
                  <FormField label="Protocol" className="allocation-protocol">
                    <Select
                      id={`allocation-protocol-${index}`}
                      value={allocation.protocol}
                      onValueChange={(selectedValue) =>
                        changeAllocation(index, {
                          protocol: selectedValue === "udp" ? "udp" : "tcp",
                        })
                      }
                    >
                      <option value="tcp">TCP</option>
                      <option value="udp">UDP</option>
                    </Select>
                  </FormField>
                  <Button
                    type="button"
                    variant="danger"
                    iconOnly
                    className="allocation-remove"
                    aria-label={`Remove range ${index + 1}`}
                    title={`Remove range ${index + 1}`}
                    onClick={() =>
                      setSettings({
                        ...settings,
                        port_allocations: settings.port_allocations.filter(
                          (_, i) => i !== index,
                        ),
                      })
                    }
                  >
                    <svg
                      width="16"
                      height="16"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="1.8"
                      strokeLinecap="round"
                      aria-hidden="true"
                    >
                      <path d="M6 6l12 12M18 6 6 18" />
                    </svg>
                  </Button>
                </fieldset>
              ))}
              <Button
                type="button"
                disabled={settings.port_allocations.length >= 128}
                onClick={() =>
                  setSettings({
                    ...settings,
                    port_allocations: [
                      ...settings.port_allocations,
                      {
                        bind_ip: "0.0.0.0",
                        start: 25565,
                        end: 25565,
                        protocol: "tcp",
                      },
                    ],
                  })
                }
              >
                + Add port range
              </Button>
            </div>
          ),
        },
        {
          id: "access",
          title: "Access",
          validate: () =>
            rolesLoading
              ? "Wait for server creation access to load."
              : rolesError,
          content: (
            <fieldset>
              <legend>Server creation access</legend>
              <p className="field-help">
                Choose which users or roles can create servers on this Spark.
                Administrators always have access.
              </p>
              {rolesLoading ? (
                <p role="status">Loading roles…</p>
              ) : rolesError ? (
                <>
                  <InlineMessage>{rolesError}</InlineMessage>
                  <Button
                    type="button"
                    onClick={() => {
                      setRolesLoading(true);
                      setRolesError(null);
                      setRolesAttempt((attempt) => attempt + 1);
                    }}
                  >
                    Retry
                  </Button>
                </>
              ) : (
                <AccessGrantFields
                  users={users}
                  grants={grants}
                  subjectRoles={roles}
                  accessRoles={[{ id: 2, name: "Create servers" }]}
                  onChange={(next, removedID) => {
                    setGrants(next);
                    if (removedID) setRemovedIDs([...removedIDs, removedID]);
                  }}
                />
              )}
              <p className="field-help">
                This does not grant Spark management access. Removing access
                does not affect ownership of existing servers.
              </p>
            </fieldset>
          ),
        },
        {
          id: "review",
          title: "Review",
          content: (
            <SparkReview
              name={name}
              description={description}
              disabled={disabled}
              settings={settings}
              serverCreationRoles={[
                "Administrators",
                ...grants.map((grant) =>
                  grant.role_id === null
                    ? grant.username
                    : `Role: ${grant.role_name}`,
                ),
              ].join(", ")}
            />
          ),
        },
      ]}
    >
      {(content, actions) => (
        <Drawer
          title={spark ? "Edit Spark" : "Add Spark"}
          description={
            spark
              ? "Manage this node’s capacity, network, and availability."
              : "Register a node to host your game servers."
          }
          busy={submitting}
          onClose={onClose}
          footerLeft={
            <Button
              type="button"

              disabled={submitting}
              onClick={onClose}
            >
              Cancel
            </Button>
          }
          footerRight={actions}
        >
          {content}
        </Drawer>
      )}
    </Wizard>
  );
}
