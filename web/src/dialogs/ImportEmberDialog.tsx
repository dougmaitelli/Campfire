import { Fieldset } from "../components/forms/Fieldset";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import "./import-ember-dialog.css";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { Textarea } from "../components/forms/Textarea";
import { Select } from "../components/forms/Select";
import { useEffect, useId, useRef, useState } from "react";
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

const PREVIEW_DEBOUNCE_MS = 350;

interface DefinitionPreview {
  definition: string;
  startupCommand: string;
  sourceURL: string | undefined;
  preview: EmberPreview | null;
  error: string | null;
}

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
  const [startupCommand, setStartupCommand] = useState(
    initialPreview?.selected_startup_command ?? "",
  );
  const [previewResult, setPreviewResult] = useState<DefinitionPreview | null>(
    initialPreview
      ? {
          definition: text,
          startupCommand,
          sourceURL,
          preview: initialPreview,
          error: null,
        }
      : null,
  );
  const sameDefinition =
    previewResult?.definition === text && previewResult.sourceURL === sourceURL;
  const currentResult =
    sameDefinition && previewResult.startupCommand === startupCommand
      ? previewResult
      : null;
  const preview = currentResult?.preview ?? null;
  const previewError = currentResult?.error ?? null;
  const checking = !!text.trim() && !loadingSource && !currentResult;
  const startupChoices = sameDefinition
    ? (previewResult.preview?.startup_commands ?? [])
    : [];
  const [reviewed, setReviewed] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const warnings =
    preview?.warnings.filter((issue) => !issue.informational) ?? [];
  const submitting = useRef(false);
  const urlRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!checking) return;
    const controller = new AbortController();
    const timer = setTimeout(() => {
      void previewEmberDefinition(
        text,
        startupCommand,
        sourceURL,
        controller.signal,
      )
        .then((result) => {
          if (!controller.signal.aborted)
            setPreviewResult({
              definition: text,
              startupCommand,
              sourceURL,
              preview: result,
              error: null,
            });
        })
        .catch((cause: unknown) => {
          if (!controller.signal.aborted)
            setPreviewResult({
              definition: text,
              startupCommand,
              sourceURL,
              preview: null,
              error:
                cause instanceof Error
                  ? cause.message
                  : "Unable to check compatibility.",
            });
        });
    }, PREVIEW_DEBOUNCE_MS);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [checking, text, startupCommand, sourceURL]);

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
    setPreviewResult(null);
    setStartupCommand("");
    setReviewed(false);
    setBusy(true);
    setError(null);
    try {
      const result = await previewEmberURL(url.trim());
      setSourceURL(url.trim());
      setText(result.definition);
      const selected = result.selected_startup_command ?? "";
      setStartupCommand(selected);
      setPreviewResult({
        definition: result.definition,
        startupCommand: selected,
        sourceURL: url.trim(),
        preview: result,
        error: null,
      });
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
      setPreviewResult(null);
      setReviewed(false);
      setStartupCommand("");
      setError("Definition files must be at most 1 MiB.");
      return;
    }
    if (submitting.current) return;
    submitting.current = true;
    setLoadingSource(true);
    setText("");
    setPreviewResult(null);
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
          {(error ?? previewError) && (
            <InlineMessage role="alert">{error ?? previewError}</InlineMessage>
          )}
          <Fieldset disabled={busy}>
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
                  setError(null);
                  setReviewed(false);
                  setStartupCommand("");
                }}
                spellCheck={false}
              />
            </FormField>
            {startupChoices.length > 1 && (
              <>
                <FormField label="Startup command to import">
                  <Select
                    id="egg-startup-command"
                    value={startupCommand}
                    onValueChange={(selectedValue) => {
                      setStartupCommand(selectedValue);
                      setReviewed(false);
                      setError(null);
                    }}
                  >
                    <option value="">Select a startup command</option>
                    {startupChoices.map((command) => (
                      <option key={command} value={command}>
                        {command}
                      </option>
                    ))}
                  </Select>
                </FormField>
              </>
            )}
            {checking && <p role="status">Checking compatibility…</p>}
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
          </Fieldset>
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
