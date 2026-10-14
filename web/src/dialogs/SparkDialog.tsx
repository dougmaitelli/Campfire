import { useEffect, useRef, useState } from "react";
import { createSpark, updateSpark } from "../api/sparks";
import type { Spark, SparkKind } from "../api/sparks";
import { SparkSettingsFields } from "../components/sparks/SparkSettingsFields";
import type { SparkSettings } from "../api/sparks";
import { Wizard } from "../components/wizard/Wizard";
import { SparkReview } from "../components/sparks/SparkReview";

interface SparkDialogProps {
  spark: Spark | null;
  hasLocalSpark: boolean;
  onClose: () => void;
  onSaved: (spark: Spark) => void;
}

export function SparkDialog({
  spark,
  hasLocalSpark,
  onClose,
  onSaved,
}: SparkDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const submittingRef = useRef(false);
  const [name, setName] = useState(spark?.name ?? "");
  const [kind, setKind] = useState<SparkKind>(spark?.kind ?? "remote");
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

  useEffect(() => {
    const dialog = dialogRef.current;
    dialog?.showModal();
    dialog?.querySelector<HTMLInputElement>("input")?.focus();
    return () => dialog?.close();
  }, []);

  async function submit() {
    if (submittingRef.current) return;
    submittingRef.current = true;
    setSubmitting(true);
    setError(null);
    try {
      const result = spark
        ? await updateSpark(spark.id, { description, disabled, ...settings })
        : await createSpark({
            name: name.trim(),
            kind,
            description,
            disabled,
            ...settings,
          });
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
    <dialog
      ref={dialogRef}
      className="spark-drawer"
      aria-labelledby="add-spark-title"
      aria-describedby="add-spark-description"
      onCancel={(event) => {
        event.preventDefault();
        if (!submittingRef.current) onClose();
      }}
    >
      <div className="spark-form">
        <header className="drawer-header">
          <h2 id="add-spark-title">{spark ? "Edit Spark" : "Add Spark"}</h2>
          <button
            type="button"
            className="button button-icon"
            aria-label="Close"
            disabled={submitting}
            onClick={onClose}
          >
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.8"
              strokeLinecap="round"
              aria-hidden="true"
            >
              <path d="M6 6l12 12M18 6 6 18" />
            </svg>
          </button>
        </header>
        <div className="spark-introduction">
          <p id="add-spark-description" className="muted">
            {spark
              ? "Manage this node’s capacity, game networking, and availability."
              : "Register a node to host your game servers."}
          </p>
        </div>
        <Wizard
          allowAllSteps={spark !== null}
          busy={submitting}
          error={error}
          finishLabel={spark ? "Save changes" : "Add Spark"}
          onFinish={submit}
          onCancel={onClose}
          steps={[
            {
              id: "basics",
              title: "Basics",
              validate: () =>
                name.trim() === "" ? "Enter a Spark name." : null,
              content: (
                <>
                  <label htmlFor="spark-name">Name</label>
                  <input
                    id="spark-name"
                    name="name"
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                    required
                    maxLength={100}
                    placeholder="e.g. Home server"
                    autoComplete="off"
                    readOnly={spark !== null}
                  />
                  <label htmlFor="spark-kind">Type</label>
                  <select
                    id="spark-kind"
                    name="kind"
                    value={kind}
                    disabled={spark !== null}
                    onChange={(event) =>
                      setKind(
                        event.target.value === "local" ? "local" : "remote",
                      )
                    }
                    aria-describedby="spark-kind-help"
                  >
                    <option value="remote">Remote Spark</option>
                    <option value="local" disabled={hasLocalSpark}>
                      Local Spark
                    </option>
                  </select>
                  <p id="spark-kind-help" className="field-help">
                    {kind === "local"
                      ? "Runs on the same host as Campfire. No separate agent is needed."
                      : "Runs on another host using the Spark agent."}
                    {hasLocalSpark && " A local Spark is already registered."}
                  </p>
                  <label htmlFor="spark-description">
                    Description <span className="field-help">(optional)</span>
                  </label>
                  <textarea
                    id="spark-description"
                    name="description"
                    value={description}
                    onChange={(event) => setDescription(event.target.value)}
                    maxLength={1000}
                    rows={4}
                    placeholder="Notes about this node"
                  />
                  <label className="checkbox-field" htmlFor="spark-disabled">
                    <input
                      type="checkbox"
                      id="spark-disabled"
                      checked={disabled}
                      onChange={(event) => setDisabled(event.target.checked)}
                    />
                    Disabled
                  </label>
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
              id: "review",
              title: "Review",
              content: (
                <SparkReview
                  name={name}
                  kind={kind}
                  description={description}
                  disabled={disabled}
                  settings={settings}
                />
              ),
            },
          ]}
        />
      </div>
    </dialog>
  );
}
