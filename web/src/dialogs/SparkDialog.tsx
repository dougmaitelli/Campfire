import { useRef, useState } from "react";
import { createSpark, updateSpark } from "../api/sparks";
import type { Spark } from "../api/sparks";
import { SparkSettingsFields } from "../components/sparks/SparkSettingsFields";
import type { SparkSettings } from "../api/sparks";
import { Wizard } from "../components/wizard/Wizard";
import { SparkReview } from "../components/sparks/SparkReview";
import { Drawer } from "../components/layout/Drawer";

interface SparkDialogProps {
  spark: Spark | null;
  onClose: () => void;
  onSaved: (spark: Spark) => void;
}

export function SparkDialog({ spark, onClose, onSaved }: SparkDialogProps) {
  const submittingRef = useRef(false);
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
    submittingRef.current = true;
    setSubmitting(true);
    setError(null);
    try {
      const result = spark
        ? await updateSpark(spark.id, { description, disabled, ...settings })
        : await createSpark({
            name: name.trim(),
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
    <Wizard
      allowAllSteps={spark !== null}
      busy={submitting}
      error={error}
      finishLabel={spark ? "Save changes" : "Add Spark"}
      onFinish={submit}
      steps={[
        {
          id: "basics",
          title: "Basics",
          validate: () => (name.trim() === "" ? "Enter a Spark name." : null),
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
              <p className="field-help">
                Run the Spark agent on this host or another machine, then enroll
                it with Campfire.
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
              description={description}
              disabled={disabled}
              settings={settings}
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
            <button
              type="button"
              className="button"
              disabled={submitting}
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
