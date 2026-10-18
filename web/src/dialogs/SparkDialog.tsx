import { FormField } from "../components/forms/FormField";
import { TextInput } from "../components/forms/TextInput";
import { Textarea } from "../components/forms/Textarea";
import { Button } from "../components/forms/Button";
import { useEffect, useRef, useState } from "react";
import {
  listRoles,
  readSparkRoles,
  setSparkRoles,
  type Role,
} from "../api/access";
import { InlineMessage } from "../components/status/InlineMessage";
import { createSpark, updateSpark } from "../api/sparks";
import type { Spark } from "../api/sparks";
import { SparkSettingsFields } from "../components/sparks/SparkSettingsFields";
import type { SparkSettings } from "../api/sparks";
import { Wizard } from "../components/wizard/Wizard";
import { SparkReview } from "../components/sparks/SparkReview";
import { Drawer } from "../components/layout/Drawer";
import { CheckboxField } from "../components/forms/CheckboxField";

interface SparkDialogProps {
  spark: Spark | null;
  onClose: () => void;
  onSaved: (spark: Spark) => void;
}

export function SparkDialog({ spark, onClose, onSaved }: SparkDialogProps) {
  const submittingRef = useRef(false);
  const [created, setCreated] = useState<Spark | null>(null);
  const [roles, setRoles] = useState<Role[]>([]);
  const [selectedRoles, setSelectedRoles] = useState<number[]>([]);
  const [rolesLoading, setRolesLoading] = useState(true);
  const [rolesError, setRolesError] = useState<string | null>(null);
  const [rolesAttempt, setRolesAttempt] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    void Promise.all([
      listRoles(controller.signal),
      spark ? readSparkRoles(spark.id, controller.signal) : Promise.resolve([]),
    ])
      .then(([catalog, ids]) => {
        if (controller.signal.aborted) return;
        setRoles(
          catalog.filter(
            (role) =>
              role.id !== 1 &&
              role.scope === "platform" &&
              role.permissions.includes("servers.create"),
          ),
        );
        setSelectedRoles(ids);
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
        await setSparkRoles(
          result.id,
          selectedRoles.filter((id) => roles.some((role) => role.id === id)),
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
            <SparkSettingsFields value={settings} onChange={setSettings} />
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
                Choose which roles can create servers on this Spark.
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
              ) : roles.length === 0 ? (
                <p className="field-help">
                  No other roles have server creation permission.
                </p>
              ) : (
                roles.map((role) => (
                  <CheckboxField
                    key={role.id}
                    checked={selectedRoles.includes(role.id)}
                    onChange={(event) =>
                      setSelectedRoles(
                        event.target.checked
                          ? [...selectedRoles, role.id]
                          : selectedRoles.filter((id) => id !== role.id),
                      )
                    }
                  >
                    {role.name}
                  </CheckboxField>
                ))
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
                ...roles
                  .filter((role) => selectedRoles.includes(role.id))
                  .map((role) => role.name),
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
              ? "Manage this node’s capacity, game networking, and availability."
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
