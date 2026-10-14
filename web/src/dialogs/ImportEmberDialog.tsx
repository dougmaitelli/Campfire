import { useId, useRef, useState } from "react";
import type { ChangeEvent, FormEvent } from "react";
import { importEmber, previewEmberURL } from "../api/embers";
import type { Ember } from "../api/embers";
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
    setBusy(true);
    setError(null);
    try {
      setText(JSON.stringify(await previewEmberURL(url.trim()), null, 2));
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
      setError("Ember files must be at most 1 MiB.");
      return;
    }
    if (submitting.current) return;
    submitting.current = true;
    setLoadingSource(true);
    setText("");
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
    if (submitting.current) return;
    submitting.current = true;
    setBusy(true);
    setError(null);
    try {
      const definition: unknown = JSON.parse(text);
      const ember = await importEmber(definition);
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
  return (
    <Drawer
      title="Import Ember"
      description="Import a native game definition. Each import creates an immutable revision."
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
          disabled={busy || text.trim() === ""}
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
                      : "Paste JSON"}
                </button>
              ))}
            </div>
            {source === "file" && (
              <>
                <label htmlFor="ember-file">JSON file</label>
                <input
                  id="ember-file"
                  type="file"
                  accept=".json,application/json"
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
                  Use a public HTTPS link to the raw JSON file, then review the
                  definition below.
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
              onChange={(event) => setText(event.target.value)}
              placeholder={example}
              spellCheck={false}
            />
            <button
              type="button"
              className="button"
              onClick={() => {
                setSource("paste");
                setText(example);
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
