import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { Fieldset } from "../components/forms/Fieldset";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import "./import-ember-drawer.css";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { Textarea } from "../components/forms/Textarea";
import { useEffect, useId, useRef, useState } from "react";
import type { ChangeEvent, FormEvent, ReactNode } from "react";
import {
  importEmber,
  deleteEmber,
  previewEmberURL,
  previewEmberDefinition,
} from "../api/embers";
import type { Ember, EmberPreview } from "../api/embers";
import { Drawer } from "../components/layout/Drawer";
import { ConfirmDialog } from "../components/layout/ConfirmDialog";
import { Tabs } from "../components/navigation/Tabs";
import { DrawerDeleteButton } from "../components/layout/DrawerDeleteButton";
import { EmberEditor } from "../components/embers/EmberEditor";
import { EmberCompatibilityPreview } from "../components/embers/EmberCompatibilityPreview";
import {
  createEmberDraft,
  emptyEmber,
  emberDraftIssue,
  serializeEmberDraft,
  type EmberDraft,
} from "../components/embers/emberDraft";

const PREVIEW_DEBOUNCE_MS = 350;

interface DefinitionPreview {
  definition: string;
  sourceURL: string | undefined;
  preview: EmberPreview | null;
  error: string | null;
}

export function ImportEmberDrawer({
  ember,
  initialPreview,
  initialSourceURL,
  initialMode,
  onClose,
  onImported,
  onDeleted,
}: {
  ember?: Ember | null;
  initialPreview?: EmberPreview | undefined;
  initialSourceURL?: string | undefined;
  initialMode?: "visual" | "json" | undefined;
  onClose: () => void;
  onImported: (ember: Ember) => void;
  onDeleted?: () => void;
}) {
  const { t } = useTranslation();
  const startsVisual =
    !initialPreview &&
    (initialMode ?? (ember ? "visual" : "json")) === "visual";
  const creating = !ember && initialMode === "visual";
  const [mode, setMode] = useState<"visual" | "json">(
    startsVisual ? "visual" : "json",
  );
  const [draft, setDraft] = useState<EmberDraft | null>(() =>
    startsVisual ? createEmberDraft(ember ?? emptyEmber()) : null,
  );
  const [text, setText] = useState(
    () =>
      initialPreview?.definition ??
      (ember
        ? JSON.stringify(ember, null, 2)
        : startsVisual
          ? JSON.stringify(emptyEmber(), null, 2)
          : ""),
  );
  const formID = useId();
  const modeID = useId();
  const [url, setURL] = useState("");
  const [sourceURL, setSourceURL] = useState<string | undefined>(
    initialSourceURL,
  );
  const [busy, setBusy] = useState(false);
  const [loadingSource, setLoadingSource] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [conversionWarnings, setConversionWarnings] = useState<{
    origin: string | undefined;
    warnings: EmberPreview["warnings"];
  } | null>(null);
  const [previewResult, setPreviewResult] = useState<DefinitionPreview | null>(
    initialPreview
      ? { definition: text, sourceURL, preview: initialPreview, error: null }
      : null,
  );
  const sameDefinition =
    previewResult?.definition === text && previewResult.sourceURL === sourceURL;
  const currentResult = sameDefinition ? previewResult : null;
  const preview = currentResult?.preview ?? null;
  const previewError = currentResult?.error ?? null;
  const checking = !!text.trim() && !loadingSource && !currentResult;
  const [reviewed, setReviewed] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  // Native previews no longer produce the source egg's conversion warnings.
  // Keep them across edits while the definition retains the same import metadata.
  const retainedWarnings =
    conversionWarnings &&
    JSON.stringify(preview?.ember?.origin) === conversionWarnings.origin
      ? conversionWarnings.warnings
      : [];
  const warnings = [...retainedWarnings, ...(preview?.warnings ?? [])].filter(
    (issue, index, items) =>
      !issue.informational &&
      items.findIndex(
        (other) =>
          other.field === issue.field && other.message === issue.message,
      ) === index,
  );
  const draftIssue = draft ? emberDraftIssue(draft) : null;
  const draftMatchesText =
    mode === "json" &&
    !!draft &&
    !draftIssue &&
    text === JSON.stringify(serializeEmberDraft(draft), null, 2);
  const ready =
    !!preview?.ember &&
    !preview.errors.length &&
    (!warnings.length || reviewed) &&
    (mode !== "visual" || !draftIssue);
  const submitting = useRef(false);
  const urlRef = useRef<HTMLInputElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!checking) return;
    const controller = new AbortController();
    const timer = setTimeout(() => {
      void previewEmberDefinition(text, sourceURL, controller.signal)
        .then((result) => {
          if (!controller.signal.aborted)
            setPreviewResult({
              definition: text,
              sourceURL,
              preview: result,
              error: null,
            });
        })
        .catch((cause: unknown) => {
          if (!controller.signal.aborted)
            setPreviewResult({
              definition: text,
              sourceURL,
              preview: null,
              error:
                cause instanceof Error
                  ? cause.message
                  : i18n.t("importEmberDrawer.unableToCheckCompatibility"),
            });
        });
    }, PREVIEW_DEBOUNCE_MS);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [checking, text, sourceURL]);

  function showVisual() {
    if (mode === "visual") return;
    setError(null);
    if (draftMatchesText) {
      setMode("visual");
      return;
    }
    const definition = preview?.ember ?? (!text.trim() ? emptyEmber() : null);
    if (!definition) {
      setError(t("emberEditor.visualUnavailable"));
      return;
    }
    if (preview && preview.format !== "ember")
      setConversionWarnings({
        origin: JSON.stringify(definition.origin),
        warnings: preview.warnings,
      });
    setDraft(createEmberDraft(definition));
    if (!text.trim()) setText(JSON.stringify(definition, null, 2));
    setMode("visual");
  }

  function changeVisual(next: EmberDraft) {
    setDraft(next);
    setError(null);
    setReviewed(false);
    const definition = next.definition;
    if (
      sourceURL !== undefined &&
      definition.updateUrl !== draft?.definition.updateUrl
    )
      setSourceURL(
        definition.updateUrl === "" ? undefined : definition.updateUrl,
      );
    if (emberDraftIssue(next)) return;
    setText(JSON.stringify(serializeEmberDraft(next), null, 2));
  }

  async function loadURL() {
    if (submitting.current || !urlRef.current?.reportValidity() || !url.trim())
      return;
    submitting.current = true;
    setLoadingSource(true);
    setText("");
    setDraft(null);
    setConversionWarnings(null);
    setPreviewResult(null);
    setReviewed(false);
    setBusy(true);
    setError(null);
    try {
      const result = await previewEmberURL(url.trim());
      setSourceURL(url.trim());
      setText(result.definition);
      setPreviewResult({
        definition: result.definition,
        sourceURL: url.trim(),
        preview: result,
        error: null,
      });
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : t("importEmberDrawer.unableToLoadURL"),
      );
    } finally {
      submitting.current = false;
      setLoadingSource(false);
      setBusy(false);
    }
  }

  async function readFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file || submitting.current) return;
    setSourceURL(undefined);
    setDraft(null);
    setConversionWarnings(null);
    if (file.size > 1024 * 1024) {
      setText("");
      setPreviewResult(null);
      setReviewed(false);
      setError(t("importEmberDrawer.definitionFilesMustBeAtMost1MiB"));
      return;
    }
    submitting.current = true;
    setLoadingSource(true);
    setText("");
    setPreviewResult(null);
    setReviewed(false);
    setBusy(true);
    try {
      setText(await file.text());
      setError(null);
    } catch {
      setError(t("importEmberDrawer.unableToReadThisFile"));
    } finally {
      submitting.current = false;
      setLoadingSource(false);
      setBusy(false);
    }
  }

  async function submit(event?: FormEvent<HTMLFormElement>) {
    event?.preventDefault();
    if (submitting.current || !ready) return;
    submitting.current = true;
    setBusy(true);
    setError(null);
    try {
      const saved = await importEmber(text, reviewed, sourceURL, ember?.id);
      onImported(saved);
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : t("importEmberDrawer.unableToImportEmber"),
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
        cause instanceof Error
          ? cause.message
          : t("importEmberDrawer.unableToDeleteEmber"),
      );
    } finally {
      submitting.current = false;
      setBusy(false);
    }
  }

  const finishLabel = ember
    ? t("importEmberDrawer.saveEmber")
    : creating
      ? t("emberEditor.create")
      : t("importEmberDrawer.importEmber");
  const compatibility = (
    <EmberCompatibilityPreview
      preview={preview}
      checking={checking}
      warnings={warnings}
      reviewed={reviewed}
      onReviewed={setReviewed}
      showDefinition={mode === "json"}
    />
  );

  function renderDrawer(content: ReactNode, actions: ReactNode) {
    return (
      <Drawer
        title={
          ember
            ? t("importEmberDrawer.editEmber")
            : initialPreview
              ? t("importEmberDrawer.reviewEmberUpdate")
              : creating
                ? t("emberEditor.create")
                : t("importEmberDrawer.importEmber")
        }
        description={
          ember || initialPreview
            ? t(
                "importEmberDrawer.reviewTheDefinitionBeforeReplacingThisEmberExistingServersShow",
              )
            : creating
              ? t("emberEditor.createDescription")
              : t(
                  "importEmberDrawer.importANativeEmberOrPterodactylPelicanEggFromJSON",
                )
        }
        busy={busy}
        onClose={onClose}
        footerLeft={
          <Button type="button" disabled={busy} onClick={onClose}>
            {t("common.cancel")}
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
            {actions}
          </>
        }
      >
        <div className="ember-editor-mode">
          <Tabs
            label={t("emberEditor.editorMode")}
            value={mode}
            items={[
              {
                value: "visual",
                label: t("emberEditor.visual"),
                id: `${modeID}-visual`,
                panelID: `${modeID}-panel`,
                disabled:
                  busy || (mode === "json" && checking && !draftMatchesText),
              },
              {
                value: "json",
                label: t("emberEditor.json"),
                id: `${modeID}-json`,
                panelID: `${modeID}-panel`,
                disabled: busy || (mode === "visual" && !!draftIssue),
              },
            ]}
            onChange={(next) => {
              if (next === "visual") showVisual();
              else {
                if (mode === "visual" && draft && !draftIssue)
                  setText(JSON.stringify(serializeEmberDraft(draft), null, 2));
                setMode("json");
                setError(null);
              }
            }}
          />
        </div>
        <div
          className="ember-editor-panel"
          role="tabpanel"
          id={`${modeID}-panel`}
          aria-labelledby={`${modeID}-${mode}`}
        >
          {content}
        </div>
        {ember && confirmDelete && (
          <ConfirmDialog
            title={t("importEmberDrawer.deleteEmber")}
            description={t(
              "importEmberDrawer.permanentlyDeleteThisEmberCannotBeDeletedWhileServersReference",
              { emberName: ember.name },
            )}
            confirmLabel={t("common.delete")}
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

  const rawContent = (
    <form
      id={formID}
      onSubmit={(event) => {
        void submit(event);
      }}
    >
      <div className="panel-content">
        {(error ?? previewError) && (
          <InlineMessage role="alert">{error ?? previewError}</InlineMessage>
        )}
        <Fieldset disabled={busy}>
          {!ember && (
            <input
              ref={fileRef}
              type="file"
              hidden
              accept=".json,.yaml,.yml,application/json,application/yaml,text/yaml"
              onChange={(event) => {
                void readFile(event);
              }}
            />
          )}
          {!ember && (
            <>
              <div className="import-sources">
                <FormField label={t("importEmberDrawer.definitionURL")}>
                  <TextInput
                    ref={urlRef}
                    id="ember-url"
                    type="url"
                    form=""
                    value={url}
                    onChange={(event) => setURL(event.target.value)}
                    placeholder={t(
                      "importEmberDrawer.httpsExampleComGameEmberJson",
                    )}
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
                  disabled={!url.trim()}
                  onClick={() => {
                    void loadURL();
                  }}
                  loading={busy}
                  loadingLabel={t("common.loading")}
                >
                  {t("importEmberDrawer.loadDefinition")}
                </Button>
                <Button type="button" onClick={() => fileRef.current?.click()}>
                  {t("importEmberDrawer.uploadFile")}
                </Button>
              </div>
              <p className="field-help">
                {t("importEmberDrawer.useAPublicHTTPSLinkToTheRawJSONOr")}
              </p>
            </>
          )}
          <FormField label={t("importEmberDrawer.definition")}>
            <Textarea
              id="ember-definition"
              className="definition-editor"
              rows={24}
              required
              value={text}
              spellCheck={false}
              onChange={(event) => {
                setText(event.target.value);
                setError(null);
                setReviewed(false);
              }}
            />
          </FormField>
          {compatibility}
        </Fieldset>
      </div>
    </form>
  );
  const rawActions = (
    <Button
      type="submit"
      form={formID}
      variant="primary"
      disabled={busy || !ready}
    >
      {busy
        ? loadingSource
          ? t("common.loading")
          : ember
            ? t("importEmberDrawer.saving")
            : t("importEmberDrawer.importing")
        : finishLabel}
    </Button>
  );
  return (
    <EmberEditor
      draft={draft ?? createEmberDraft(emptyEmber())}
      onChange={changeVisual}
      busy={busy}
      error={error ?? previewError}
      ready={ready}
      editing={!!ember}
      finishLabel={finishLabel}
      onFinish={() => submit()}
      review={compatibility}
    >
      {(content, actions) =>
        renderDrawer(
          mode === "visual" ? content : rawContent,
          mode === "visual" ? actions : rawActions,
        )
      }
    </EmberEditor>
  );
}
