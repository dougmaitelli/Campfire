import { useTranslation } from "react-i18next";
import { EditorText } from "./EmberEditorFields";
import type { EmberDraft } from "./emberDraft";

export function EmberBasicsFields({
  draft,
  onChange,
}: {
  draft: EmberDraft;
  onChange: (value: EmberDraft) => void;
}) {
  const { t } = useTranslation();
  const value = draft.definition;
  function text(
    key: "name" | "description" | "artUrl" | "updateUrl",
    next: string,
  ) {
    onChange({ ...draft, definition: { ...value, [key]: next } });
  }
  return (
    <>
      <EditorText
        label={t("common.name")}
        value={value.name}
        required
        maxLength={120}
        onChange={(next) => text("name", next)}
      />
      <EditorText
        label={t("common.description")}
        value={value.description}
        multiline
        onChange={(next) => text("description", next)}
      />
      <EditorText
        label={t("emberEditor.artURL")}
        value={value.artUrl ?? ""}
        description={t("emberEditor.artHelp")}
        onChange={(next) => text("artUrl", next)}
      />
      <EditorText
        label={t("emberEditor.updateURL")}
        value={value.updateUrl ?? ""}
        type="url"
        description={t("emberEditor.updateHelp")}
        onChange={(next) => text("updateUrl", next)}
      />
    </>
  );
}
