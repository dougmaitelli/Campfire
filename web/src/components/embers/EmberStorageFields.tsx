import { useTranslation } from "react-i18next";
import { EditableList } from "../forms/EditableList";
import { EditorCheck, EditorGroup, EditorText } from "./EmberEditorFields";
import type { EmberDraft } from "./emberDraft";

export function EmberStorageFields({
  draft,
  onChange,
}: {
  draft: EmberDraft;
  onChange: (value: EmberDraft) => void;
}) {
  const { t } = useTranslation();
  const value = draft.definition;
  const paths = value.savedata_paths ?? [];
  function patch(next: Partial<typeof value>) {
    onChange({ ...draft, definition: { ...value, ...next } });
  }
  return (
    <>
      <EditorGroup
        title={t("emberEditor.savedata")}
        description={t("emberEditor.savedataHelp")}
      >
        <EditableList
          items={paths}
          maxItems={64}
          rowLabel={(index) =>
            t("emberEditor.item", {
              name: t("emberEditor.savePath"),
              number: index + 1,
            })
          }
          removeLabel={(index) =>
            t("emberEditor.removeItem", {
              name: t("emberEditor.savePath"),
              number: index + 1,
            })
          }
          addLabel={t("emberEditor.add", { name: t("emberEditor.savePath") })}
          onRemove={(index) =>
            patch({ savedata_paths: paths.filter((_, i) => i !== index) })
          }
          onAdd={() =>
            patch({ savedata_paths: [...paths, { path: "", required: false }] })
          }
          renderBody={(path, index) => (
            <EditorCheck
              label={t("emberEditor.requiredPath")}
              value={path.required}
              onChange={(required) =>
                patch({
                  savedata_paths: paths.map((item, i) =>
                    i === index ? { ...item, required } : item,
                  ),
                })
              }
            />
          )}
        >
          {(path, index) => (
            <EditorText
              label={t("emberEditor.relativePath")}
              value={path.path}
              required
              onChange={(next) =>
                patch({
                  savedata_paths: paths.map((item, i) =>
                    i === index ? { ...item, path: next } : item,
                  ),
                })
              }
            />
          )}
        </EditableList>
      </EditorGroup>
    </>
  );
}
