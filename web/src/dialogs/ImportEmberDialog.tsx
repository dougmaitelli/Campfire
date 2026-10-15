import { useId, useRef, useState } from "react";
import type { ChangeEvent, FormEvent } from "react";
import {
  importEmber,
  previewEmberURL,
  previewEmberDefinition,
} from "../api/embers";
import type { Ember, EmberPreview } from "../api/embers";
import { Drawer } from "../components/layout/Drawer";

const example = JSON.stringify(
  {
    schema: 1,
    name: "Example game",
    description: "",
    images: { Alpine: "alpine:3.22" },
    startup: "sleep 3600",
    work_dir: "/home/container",
    user: "1000:1000",
    variables: [],
    required_ports: [{ container: 25565, protocol: "tcp" }],
  },
  null,
  2,
);

export function ImportEmberDialog({
  onClose,
  onImported,
}: {
  onClose: () => void;
  onImported: (ember: Ember) => void;
}) {
  const [text, setText] = useState("");
  const formID = useId();
  const [source, setSource] = useState<"file" | "url" | "paste">("file");
  const [url, setURL] = useState("");
  const [busy, setBusy] = useState(false);
  const [loadingSource, setLoadingSource] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [preview, setPreview] = useState<EmberPreview | null>(null);
  const [startupCommand, setStartupCommand] = useState("");
  const [reviewed, setReviewed] = useState(false);
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
      (preview.warnings.length > 0 && !reviewed)
    )
      return;
    submitting.current = true;
    setBusy(true);
    setError(null);
    try {
      const ember = await importEmber(text, startupCommand, reviewed);
      onImported(ember);
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
      setPreview(await previewEmberDefinition(text, startupCommand));
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
  return (
    <Drawer
      title="Import Ember"
      description="Import a native Ember or Pterodactyl/Pelican Egg from JSON or YAML. Review compatibility before saving an immutable revision."
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
      footerRight={
        <button
          type="submit"
          form={formID}
          className="button button-primary"
          disabled={
            busy ||
            !preview?.ember ||
            preview.errors.length > 0 ||
            (preview.warnings.length > 0 && !reviewed)
          }
        >
          {busy ? (loadingSource ? "Loading…" : "Importing…") : "Import Ember"}
        </button>
      }
    >
      <form
        id={formID}
        onSubmit={(event) => {
          void submit(event);
        }}
      >
        <div className="drawer-content">
          {error && (
            <p className="error-message" role="alert">
              {error}
            </p>
          )}
          <fieldset disabled={busy}>
            <div
              className="import-sources"
              role="group"
              aria-label="Definition source"
            >
              {(["file", "url", "paste"] as const).map((option) => (
                <button
                  key={option}
                  type="button"
                  className="button"
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
                </button>
              ))}
            </div>
            {source === "file" && (
              <>
                <label htmlFor="ember-file">JSON or YAML file</label>
                <input
                  id="ember-file"
                  type="file"
                  accept=".json,.yaml,.yml,application/json,application/yaml,text/yaml"
                  onChange={(event) => {
                    void readFile(event);
                  }}
                />
              </>
            )}
            {source === "url" && (
              <>
                <label htmlFor="ember-url">Definition URL</label>
                <input
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
                <button
                  type="button"
                  className="button"
                  disabled={url.trim() === ""}
                  onClick={() => {
                    void loadURL();
                  }}
                >
                  {busy ? "Loading…" : "Load definition"}
                </button>
                <p className="field-help">
                  Use a public HTTPS link to the raw JSON or YAML file, then
                  review the definition below.
                </p>
              </>
            )}
            <label htmlFor="ember-definition">Definition</label>
            <textarea
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
              placeholder={example}
              spellCheck={false}
            />
            {preview && preview.startup_commands.length > 1 && (
              <>
                <label htmlFor="egg-startup-command">
                  Startup command to import
                </label>
                <select
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
                </select>
              </>
            )}
            <button
              type="button"
              className="button"
              disabled={!text.trim()}
              onClick={() => {
                void review();
              }}
            >
              Preview compatibility
            </button>
            {preview && (
              <section aria-label="Compatibility preview">
                <h3>Compatibility: {preview.format || "unknown format"}</h3>
                {preview.errors.length > 0 && (
                  <>
                    <p className="error-message" role="alert">
                      This definition cannot be imported yet.
                    </p>
                    <ul>
                      {preview.errors.map((issue, index) => (
                        <li key={`${issue.field}-${index}`}>
                          <strong>{issue.field}:</strong> {issue.message}
                        </li>
                      ))}
                    </ul>
                  </>
                )}
                {preview.warnings.length > 0 && (
                  <>
                    <p>Review these differences before importing:</p>
                    <ul>
                      {preview.warnings.map((issue, index) => (
                        <li key={`${issue.field}-${index}`}>
                          <strong>{issue.field}:</strong> {issue.message}
                        </li>
                      ))}
                    </ul>
                    <label className="checkbox-field">
                      <input
                        type="checkbox"
                        checked={reviewed}
                        onChange={(event) => setReviewed(event.target.checked)}
                      />
                      I have reviewed the compatibility warnings.
                    </label>
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
            <p className="field-help">
              Supported subset: PTDL v1/v2 and PLCN v1/v2/v3, common variables
              and validation, installers, simple properties and top-level JSON.
              Unsupported runtime features block import. See
              deploy/EGG-COMPATIBILITY.txt for the full support matrix. Only
              import scripts and images you trust; they run when you install or
              start a server.
            </p>
            <button
              type="button"
              className="button"
              onClick={() => {
                setSource("paste");
                setText(example);
                setPreview(null);
                setStartupCommand("");
                setReviewed(false);
                setError(null);
              }}
            >
              Use example definition
            </button>
          </fieldset>
        </div>
      </form>
    </Drawer>
  );
}
