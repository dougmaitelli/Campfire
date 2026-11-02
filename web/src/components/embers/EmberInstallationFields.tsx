import { useTranslation } from "react-i18next";
import { FormField } from "../forms/FormField";
import { Select } from "../forms/Select";
import { EditorCheck, EditorText } from "./EmberEditorFields";
import type { EmberDraft } from "./emberDraft";
import { optionalValue } from "./emberDraft";

export function EmberInstallationFields({
  draft,
  onChange,
}: {
  draft: EmberDraft;
  onChange: (value: EmberDraft) => void;
}) {
  const { t } = useTranslation();
  const value = draft.definition;
  function patch(next: Partial<typeof value>) {
    onChange({ ...draft, definition: { ...value, ...next } });
  }
  return (
    <>
      <p className="field-help">{t("emberEditor.installerHelp")}</p>
      <EditorCheck
        label={t("emberEditor.useInstaller")}
        value={!!value.installer}
        onChange={(enabled) =>
          onChange({
            ...draft,
            definition: optionalValue(
              value,
              "installer",
              enabled ? { image: "", entrypoint: "sh", script: "" } : undefined,
            ),
          })
        }
      />
      {value.installer && (
        <>
          <EditorText
            label={t("emberEditor.installerImage")}
            value={value.installer.image}
            required
            onChange={(image) =>
              patch({ installer: { ...value.installer!, image } })
            }
          />
          <FormField label={t("emberEditor.entrypoint")}>
            <Select
              value={value.installer.entrypoint}
              onValueChange={(entrypoint) =>
                patch({ installer: { ...value.installer!, entrypoint } })
              }
            >
              {["sh", "bash", "ash", "/bin/sh", "/bin/bash", "/bin/ash"].map(
                (shell) => (
                  <option key={shell} value={shell}>
                    {shell}
                  </option>
                ),
              )}
            </Select>
          </FormField>
          <EditorText
            label={t("emberEditor.installScript")}
            value={value.installer.script}
            required
            multiline
            onChange={(script) =>
              patch({ installer: { ...value.installer!, script } })
            }
          />
        </>
      )}
    </>
  );
}
