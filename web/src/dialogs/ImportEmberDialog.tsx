import i18n from "../i18n";
import { useTranslation } from "react-i18next";
import { Fieldset } from "../components/forms/Fieldset";
import { FormField } from "../components/forms/FormField";
import { Button } from "../components/forms/Button";
import "./import-ember-dialog.css";
import { InlineMessage } from "../components/status/InlineMessage";
import { TextInput } from "../components/forms/TextInput";
import { Textarea } from "../components/forms/Textarea";
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
  const { t } = useTranslation();
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
  const [previewResult, setPreviewResult] = useState<DefinitionPreview | null>(
    initialPreview
      ? {
          definition: text,
          sourceURL,
          preview: initialPreview,
          error: null,
        }
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
  const warnings =
    preview?.warnings.filter((issue) => !issue.informational) ?? [];
  const submitting = useRef(false);
  const urlRef = useRef<HTMLInputElement>(null);

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
                  : i18n.t("importEmberDialog.unableToCheckCompatibility"),
            });
        });
    }, PREVIEW_DEBOUNCE_MS);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [checking, text, sourceURL]);

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
          : i18n.t("importEmberDialog.unableToLoadURL"),
      );
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
      setError(i18n.t("importEmberDialog.definitionFilesMustBeAtMost1MiB"));
      return;
    }
    if (submitting.current) return;
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
      setError(i18n.t("importEmberDialog.unableToReadThisFile"));
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
      const saved = await importEmber(text, reviewed, sourceURL, ember?.id);
      onImported(saved);
      onClose();
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : i18n.t("importEmberDialog.unableToImportEmber"),
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
          : i18n.t("importEmberDialog.unableToDeleteEmber"),
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
          ? t("importEmberDialog.editEmber")
          : initialPreview
            ? t("importEmberDialog.reviewEmberUpdate")
            : t("importEmberDialog.importEmber")
      }
      description={
        ember || initialPreview
          ? t(
              "importEmberDialog.reviewTheDefinitionBeforeReplacingThisEmberExistingServersShow",
            )
          : t(
              "importEmberDialog.importANativeEmberOrPterodactylPelicanEggFromJSON",
            )
      }
      busy={busy}
      onClose={onClose}
      footerLeft={
        <Button
          type="button"

          disabled={busy}
          onClick={onClose}
        >
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
                ? t("common.loading")
                : ember
                  ? t("importEmberDialog.saving")
                  : t("importEmberDialog.importing")
              : ember
                ? t("importEmberDialog.saveEmber")
                : t("importEmberDialog.importEmber")}
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
                aria-label={t("importEmberDialog.definitionSource")}
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
                      ? i18n.t("importEmberDialog.uploadFile")
                      : option === "url"
                        ? i18n.t("importEmberDialog.fromURL")
                        : i18n.t("importEmberDialog.pasteDefinition")}
                  </Button>
                ))}
              </div>
            )}
            {source === "file" && (
              <>
                <FormField label={t("importEmberDialog.jsonOrYAMLFile")}>
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
                <FormField label={t("importEmberDialog.definitionURL")}>
                  <TextInput
                    ref={urlRef}
                    id="ember-url"
                    type="url"
                    value={url}
                    onChange={(event) => setURL(event.target.value)}
                    placeholder={t(
                      "importEmberDialog.httpsExampleComGameEmberJson",
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

                  disabled={url.trim() === ""}
                  onClick={() => {
                    void loadURL();
                  }}
                  loading={busy}
                  loadingLabel={t("common.loading")}
                >
                  {t("importEmberDialog.loadDefinition")}
                </Button>
                <p className="field-help">
                  {t("importEmberDialog.useAPublicHTTPSLinkToTheRawJSONOr")}
                </p>
              </>
            )}
            <FormField label={t("importEmberDialog.definition")}>
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
                }}
                spellCheck={false}
              />
            </FormField>
            {checking && (
              <p role="status">
                {t("importEmberDialog.checkingCompatibility")}
              </p>
            )}
            {preview && (
              <section aria-label={t("importEmberDialog.compatibilityPreview")}>
                <h3>
                  {preview.errors.length > 0
                    ? t("importEmberDialog.changesNeededBeforeImport")
                    : t("importEmberDialog.isReadyToImport", {
                        previewEmber:
                          preview.ember?.name ??
                          t("importEmberDialog.definition"),
                      })}
                </h3>
                {preview.errors.length > 0 && (
                  <>
                    <InlineMessage role="alert">
                      {t("importEmberDialog.thisDefinitionCannotBeImportedYet")}
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
                    <p>
                      {t("importEmberDialog.beforeRunningAServerWithThisEmber")}
                    </p>
                    <ul>
                      {warnings.map((issue, index) => (
                        <li key={`${issue.field}-${index}`}>{issue.message}</li>
                      ))}
                    </ul>
                    <CheckboxField
                      checked={reviewed}
                      onChange={(event) => setReviewed(event.target.checked)}
                    >
                      {t("importEmberDialog.iUnderstandTheseSetupRequirements")}
                    </CheckboxField>
                  </>
                )}
                {preview.ember && (
                  <details>
                    <summary>{t("importEmberDialog.convertedEmber")}</summary>
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
          title={t("importEmberDialog.deleteEmber")}
          description={t(
            "importEmberDialog.permanentlyDeleteThisEmberCannotBeDeletedWhileServersReference",
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
