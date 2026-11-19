import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  conflictActions,
  type FileConflict,
  type FileConflictAction,
  type FileConflictChoice,
} from "./fileConflicts";
import { normalizeFileName } from "./fileNames";
import { Fieldset } from "../forms/Fieldset";
import "./file-conflicts.css";
import { ExtractionAction, FileKind } from "../../contracts/domains";
import { formatBytes } from "../../utils/formatBytes";
import { CheckboxField } from "../forms/CheckboxField";
import { FormField } from "../forms/FormField";
import { RadioField } from "../forms/RadioField";
import { TextInput } from "../forms/TextInput";
import { ConfirmDialog } from "../layout/ConfirmDialog";
import { InlineMessage } from "../status/InlineMessage";

export function FileConflictDialog({
  conflict,
  reserved,
  operation,
  hasRemainingConflicts,
  onResolve,
}: {
  conflict: FileConflict;
  reserved: ReadonlySet<string>;
  operation: "upload" | "extract";
  hasRemainingConflicts: boolean;
  onResolve: (choice: FileConflictChoice | null) => void;
}) {
  const { t } = useTranslation();
  const [action, setAction] = useState<FileConflictAction>(
    ExtractionAction.Skip,
  );
  const [name, setName] = useState(conflict.suggestedName);
  const [applyToAll, setApplyToAll] = useState(false);
  const trimmed = name.trim();
  const originalName = conflict.name.split("/").filter(Boolean).at(-1);
  const invalidName =
    normalizeFileName(name) === null ||
    trimmed === originalName ||
    reserved.has(trimmed);
  const actions = conflictActions(conflict);
  const presented: FileConflictAction[] = actions.includes(
    ExtractionAction.Merge,
  )
    ? [ExtractionAction.Skip, ExtractionAction.Merge, ExtractionAction.Rename]
    : actions.includes(ExtractionAction.Extract)
      ? [
          ExtractionAction.Skip,
          ExtractionAction.Extract,
          ExtractionAction.Rename,
        ]
      : [
          ExtractionAction.Skip,
          ExtractionAction.Overwrite,
          ExtractionAction.Rename,
        ];
  return (
    <ConfirmDialog
      title={t(
        operation === "extract"
          ? "fileExtraction.resolveConflict"
          : "fileUploads.resolveConflict",
      )}
      description={t("fileUploads.conflictDescription", {
        name: conflict.name,
      })}
      confirmLabel={t("fileUploads.continue")}
      busy={false}
      disabled={action === ExtractionAction.Rename && invalidName}
      error={null}
      onClose={() => onResolve(null)}
      onConfirm={() => {
        if (action === ExtractionAction.Rename && invalidName) return;
        onResolve({
          action,
          name: trimmed,
          applyToAll: hasRemainingConflicts && applyToAll,
        });
      }}
    >
      {conflict.message && (
        <InlineMessage tone="warning">{conflict.message}</InlineMessage>
      )}
      <p>
        {conflict.kind === FileKind.Directory
          ? t("fileExtraction.incomingDirectory")
          : t("fileUploads.incoming", { size: formatBytes(conflict.size) })}
      </p>
      {conflict.existing && (
        <p>
          {conflict.existing.kind === FileKind.File
            ? t("fileUploads.existing", {
                size: formatBytes(conflict.existing.size),
                modified: new Date(
                  conflict.existing.modified_at,
                ).toLocaleString(),
              })
            : conflict.kind === FileKind.Directory &&
                conflict.existing.kind === FileKind.Directory
              ? t("fileExtraction.existingDirectory")
              : t("fileUploads.existingNotFile")}
        </p>
      )}
      {conflict.duplicate && <p>{t("fileUploads.duplicateInBatch")}</p>}
      <Fieldset className="file-conflict-actions">
        <legend>
          {t(
            conflict.kind === FileKind.Directory
              ? "fileExtraction.directoryAction"
              : "fileUploads.conflictAction",
          )}
        </legend>
        {presented.map((value) => (
          <RadioField
            key={value}
            name="file-conflict-action"
            value={value}
            checked={action === value}
            disabled={!actions.includes(value)}
            onChange={() => setAction(value)}
          >
            {value === ExtractionAction.Merge
              ? t("fileExtraction.merge")
              : value === ExtractionAction.Extract
                ? t("fileExtraction.saveIncoming")
                : t(`fileUploads.${value}`)}
          </RadioField>
        ))}
      </Fieldset>
      {action === ExtractionAction.Merge && (
        <p>{t("fileExtraction.mergeDescription")}</p>
      )}
      {action === ExtractionAction.Overwrite && (
        <InlineMessage tone="warning">
          {t("fileUploads.overwriteWarning")}
        </InlineMessage>
      )}
      {action === ExtractionAction.Rename && (
        <FormField
          label={t(
            conflict.kind === FileKind.Directory
              ? "serverFileBrowser.newFolderName"
              : "fileUploads.newName",
          )}
          error={invalidName ? t("fileUploads.chooseUnusedName") : undefined}
        >
          <TextInput
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        </FormField>
      )}
      {conflict.expiresAt && (
        <p>
          {t("fileExtraction.conflictExpiry", {
            time: new Date(conflict.expiresAt).toLocaleTimeString(),
          })}
        </p>
      )}
      {hasRemainingConflicts && (
        <CheckboxField
          checked={applyToAll}
          onChange={(event) => setApplyToAll(event.target.checked)}
        >
          {t("fileUploads.applyToAll")}
        </CheckboxField>
      )}
      {hasRemainingConflicts &&
        applyToAll &&
        action === ExtractionAction.Rename && (
          <p>{t("fileUploads.automaticNames")}</p>
        )}
    </ConfirmDialog>
  );
}
