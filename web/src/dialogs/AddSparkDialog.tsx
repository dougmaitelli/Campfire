import { useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import { createSpark } from "../api/sparks";
import type { Spark, SparkKind } from "../api/sparks";

interface AddSparkDialogProps {
  hasLocalSpark: boolean;
  onClose: () => void;
  onCreated: (spark: Spark) => void;
}

export function AddSparkDialog({
  hasLocalSpark,
  onClose,
  onCreated,
}: AddSparkDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const submittingRef = useRef(false);
  const [name, setName] = useState("");
  const [kind, setKind] = useState<SparkKind>("remote");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const dialog = dialogRef.current;
    dialog?.showModal();
    dialog?.querySelector<HTMLInputElement>("input")?.focus();
    return () => dialog?.close();
  }, []);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submittingRef.current) return;
    submittingRef.current = true;
    setSubmitting(true);
    setError(null);
    try {
      const spark = await createSpark({ name: name.trim(), kind });
      onCreated(spark);
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
      <form
        className="spark-form"
        onSubmit={(event) => {
          void submit(event);
        }}
      >
        <header className="drawer-header">
          <h2 id="add-spark-title">Add Spark</h2>
          <button
            type="button"
            className="button button-icon"
            aria-label="Close"
            disabled={submitting}
            onClick={onClose}
          >
            ×
          </button>
        </header>
        <div className="drawer-body">
          <p id="add-spark-description" className="muted">
            Register a node to host your game servers.
          </p>
          {error && (
            <p className="error-message" role="alert">
              {error}
            </p>
          )}
          <fieldset disabled={submitting}>
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
            />
            <label htmlFor="spark-kind">Type</label>
            <select
              id="spark-kind"
              name="kind"
              value={kind}
              onChange={(event) =>
                setKind(event.target.value === "local" ? "local" : "remote")
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
          </fieldset>
        </div>
        <footer className="drawer-footer">
          <button
            type="button"
            className="button"
            disabled={submitting}
            onClick={onClose}
          >
            Cancel
          </button>
          <button
            type="submit"
            className="button button-primary"
            disabled={submitting || name.trim() === ""}
          >
            {submitting ? "Adding…" : "Add Spark"}
          </button>
        </footer>
      </form>
    </dialog>
  );
}
