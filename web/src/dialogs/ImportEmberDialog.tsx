import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { Textarea } from "../components/forms/Textarea";
import { Select } from "../components/forms/Select";
import { useId, useRef, useState } from "react";
import type { ChangeEvent, FormEvent } from "react";
import {
  importEmber,
  deleteEmber,
  previewEmberURL,
  previewEmberDefinition,
} from "../api/embers";
import type { Ember, EmberPreview } from "../api/embers";
import { Drawer } from "../components/layout/Drawer";
import { CheckboxField } from "../components/forms/CheckboxField";
import { ConfirmDialog } from "../components/layout/ConfirmDialog";
import { DrawerDeleteButton } from "../components/layout/DrawerDeleteButton";

export function ImportEmberDialog({
  ember,
  initialPreview,
  initialSourceURL,
  onClose,
  onImported,
  onDeleted,
}: {
  ember?: Ember | null;
  initialPreview?: EmberPreview | undefined;
  initialSourceURL?: string | undefined;
  onClose: () => void;
  onImported: (ember: Ember) => void;
  onDeleted?: () => void;
}) {
  const [text, setText] = useState(
    () =>
      initialPreview?.definition ??
      (ember ? JSON.stringify(ember, null, 2) : ""),
  );
  const formID = useId();
  const [source, setSource] = useState<"file" | "url" | "paste">(
    ember || initialPreview ? "paste" : "file",
  );
  const [url, setURL] = useState("");
  const [sourceURL, setSourceURL] = useState<string | undefined>(
    initialSourceURL,
  );
  const [busy, setBusy] = useState(false);
  const [loadingSource, setLoadingSource] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [preview, setPreview] = useState<EmberPreview | null>(
    initialPreview ?? null,
  );
  const [startupCommand, setStartupCommand] = useState(
    initialPreview?.selected_startup_command ?? "",
  );
  const [reviewed, setReviewed] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const warnings =
    preview?.warnings.filter((issue) => !issue.informational) ?? [];
  const notes = preview?.warnings.filter((issue) => issue.informational) ?? [];
  const submitting = useRef(false);
  const urlRef = useRef<HTMLInputElement>(null);
  async function loadURL() {
    if (
      submitting.current ||
      !urlRef.current?.reportValidity() ||
      url.trim() === ""
    )
      return;
    submitting.current = true;
    setLoadingSource(true);
    setText("");
    setPreview(null);
    setStartupCommand("");
    setReviewed(false);
    setBusy(true);
    setError(null);
    try {
      const result = await previewEmberURL(url.trim());
      setSourceURL(url.trim());
      setText(result.definition);
      setPreview(result);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Unable to load URL.");
    } finally {
      submitting.current = false;
      setLoadingSource(false);
      setBusy(false);
    }
  }
  async function readFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    if (!file) return;
    setSourceURL(undefined);
    if (file.size > 1024 * 1024) {
      setText("");
      setPreview(null);
      setReviewed(false);
      setStartupCommand("");
      setError("Definition files must be at most 1 MiB.");
      return;
    }
    if (submitting.current) return;
    submitting.current = true;
    setLoadingSource(true);
    setText("");
    setPreview(null);
    setStartupCommand("");
    setReviewed(false);
    setBusy(true);
    try {
      setText(await file.text());
      setError(null);
    } catch {
      setError("Unable to read this file.");
    } finally {
      submitting.current = false;
      setLoadingSource(false);
      setBusy(false);
    }
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (
      submitting.current ||
      !preview?.ember ||
      preview.errors.length > 0 ||
      (warnings.length > 0 && !reviewed)
    )
      return;
    submitting.current = true;
    setBusy(true);
    setError(null);
    try {
      const saved = await importEmber(
        text,
        startupCommand,
        reviewed,
        sourceURL,
        ember?.id,
      );
      onImported(saved);
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to import Ember.",
      );
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }
  async function review() {
    if (submitting.current || !text.trim()) return;
    submitting.current = true;
    setBusy(true);
    setReviewed(false);
    setError(null);
    try {
      setPreview(await previewEmberDefinition(text, startupCommand, sourceURL));
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to preview definition.",
      );
      setPreview(null);
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }
  async function remove() {
    if (!ember || submitting.current) return;
    submitting.current = true;
    setBusy(true);
    setError(null);
    try {
      await deleteEmber(ember.id);
      onDeleted?.();
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "Unable to delete Ember.",
      );
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={
        ember
          ? "Edit Ember"
          : initialPreview
            ? "Review Ember update"
            : "Import Ember"
      }
      description={
        ember || initialPreview
          ? "Review the definition before replacing this Ember. Existing servers show an update until you apply it."
          : "Import a native Ember or Pterodactyl/Pelican Egg from JSON or YAML. Review compatibility before importing."
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
      footerRight={
        <>
          {ember && (
            <DrawerDeleteButton
              disabled={busy}
              deleting={busy && confirmDelete}
              onDelete={() => {
                setError(null);
                setConfirmDelete(true);
              }}
            />
          )}
          <Button
            type="submit"
            form={formID}
            variant="primary"
            disabled={
              busy ||
              !preview?.ember ||
              preview.errors.length > 0 ||
              (warnings.length > 0 && !reviewed)
            }
          >
            {busy
              ? loadingSource
                ? "Loading…"
                : ember
                  ? "Saving…"
                  : "Importing…"
              : ember
                ? "Save Ember"
                : "Import Ember"}
          </Button>
        </>
      }
    >
      <form
        id={formID}
        onSubmit={(event) => {
          void submit(event);
        }}
      >
        <div className="drawer-content">
          {error && <InlineMessage role="alert">{error}</InlineMessage>}
          <fieldset disabled={busy}>
            {!ember && (
              <div
                className="import-sources"
                role="group"
                aria-label="Definition source"
              >
                {(["file", "url", "paste"] as const).map((option) => (
                  <Button
                    key={option}
                    type="button"

                    aria-pressed={source === option}
                    onClick={() => {
                      setSource(option);
                      setError(null);
                    }}
                  >
                    {option === "file"
                      ? "Upload file"
                      : option === "url"
                        ? "From URL"
                        : "Paste definition"}
                  </Button>
                ))}
              </div>
            )}
            {source === "file" && (
              <>
                <FormField label="JSON or YAML file">
                  <input
                    id="ember-file"
                    type="file"
                    accept=".json,.yaml,.yml,application/json,application/yaml,text/yaml"
                    onChange={(event) => {
                      void readFile(event);
                    }}
                  />
                </FormField>
              </>
            )}
            {source === "url" && (
              <>
                <FormField label="Definition URL">
                  <TextInput
                    ref={urlRef}
                    id="ember-url"
                    type="url"
                    value={url}
                    onChange={(event) => setURL(event.target.value)}
                    placeholder="https://example.com/game.ember.json"
                    onKeyDown={(event) => {
                      if (event.key === "Enter") {
                        event.preventDefault();
                        void loadURL();
                      }
                    }}
                  />
                </FormField>
                <Button
                  type="button"

                  disabled={url.trim() === ""}
                  onClick={() => {
                    void loadURL();
                  }}
                  loading={busy}
                  loadingLabel={"Loading…"}
                >
                  {"Load definition"}
                </Button>
                <p className="field-help">
                  Use a public HTTPS link to the raw JSON or YAML file, then
                  review the definition below.
                </p>
              </>
            )}
            <FormField label="Definition">
              <Textarea
                id="ember-definition"
                className="definition-editor"
                rows={24}
                required
                value={text}
                onChange={(event) => {
                  setText(event.target.value);
                  setPreview(null);
                  setReviewed(false);
                  setStartupCommand("");
                }}
                spellCheck={false}
              />
            </FormField>
            {preview && preview.startup_commands.length > 1 && (
              <>
                <FormField label="Startup command to import">
                  <Select
                    id="egg-startup-command"
                    value={startupCommand}
                    onChange={(event) => {
                      setStartupCommand(event.target.value);
                      setReviewed(false);
                      setPreview({ ...preview, ember: null });
                    }}
                  >
                    <option value="">Select a startup command</option>
                    {preview.startup_commands.map((command) => (
                      <option key={command} value={command}>
                        {command}
                      </option>
                    ))}
                  </Select>
                </FormField>
              </>
            )}
            <Button
              type="button"

              disabled={!text.trim()}
              onClick={() => {
                void review();
              }}
            >
              Preview compatibility
            </Button>
            {preview && (
              <section aria-label="Compatibility preview">
                <h3>
                  {preview.errors.length > 0
                    ? "Changes needed before import"
                    : `${preview.ember?.name ?? "Definition"} is ready to import`}
                </h3>
                {preview.errors.length > 0 && (
                  <>
                    <InlineMessage role="alert">
                      This definition cannot be imported yet.
                    </InlineMessage>
                    <ul>
                      {preview.errors.map((issue, index) => (
                        <li key={`${issue.field}-${index}`}>
                          <strong>{issue.field}:</strong> {issue.message}
                        </li>
                      ))}
                    </ul>
                  </>
                )}
                {warnings.length > 0 && (
                  <>
                    <p>Before running a server with this Ember:</p>
                    <ul>
                      {warnings.map((issue, index) => (
                        <li key={`${issue.field}-${index}`}>{issue.message}</li>
                      ))}
                    </ul>
                    <CheckboxField
                      checked={reviewed}
                      onChange={(event) => setReviewed(event.target.checked)}
                    >
                      I understand these setup requirements.
                    </CheckboxField>
                  </>
                )}
                {notes.length > 0 && (
                  <details>
                    <summary>Conversion details ({notes.length})</summary>
                    <ul>
                      {notes.map((issue, index) => (
                        <li key={`${issue.field}-${index}`}>
                          <strong>{issue.field}:</strong> {issue.message}
                        </li>
                      ))}
                    </ul>
                  </details>
                )}
                {preview.ember && (
                  <details>
                    <summary>Converted Ember</summary>
                    <pre className="ember-preview-definition">
                      {JSON.stringify(preview.ember, null, 2)}
                    </pre>
                  </details>
                )}
              </section>
            )}
            <p className="field-help">
              Supported subset: PTDL v1/v2 and PLCN v1/v2/v3, common variables
              and validation, installers, simple properties and top-level JSON.
              Unsupported runtime features block import. See
              deploy/EGG-COMPATIBILITY.txt for the full support matrix. Only
              import scripts and images you trust; they run when you install or
              start a server.
            </p>
          </fieldset>
        </div>
      </form>
      {ember && confirmDelete && (
        <ConfirmDialog
          title="Delete Ember?"
          description={`Permanently delete "${ember.name}"? This Ember cannot be deleted while servers reference it.`}
          confirmLabel="Delete"
          danger
          busy={busy}
          error={error}
          onClose={() => setConfirmDelete(false)}
          onConfirm={() => {
            void remove();
          }}
        />
      )}
    </Drawer>
  );
}
