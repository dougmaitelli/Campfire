import { useTranslation } from "react-i18next";
import type { EmberConfigFile } from "../../api/embers";
import { ConfigParser } from "../../contracts/domains";
import { FormField } from "../forms/FormField";
import { Select } from "../forms/Select";
import { AllocationList } from "../forms/AllocationList";
import {
  EditorCheck,
  EditorText,
  ReplacementValues,
} from "./EmberEditorFields";
import type { EmberDraft, EmberFileDraft } from "./emberDraft";

export function EmberFileFields({
  draft,
  onChange,
}: {
  draft: EmberDraft;
  onChange: (value: EmberDraft) => void;
}) {
  const { t } = useTranslation();
  function change(index: number, next: EmberFileDraft) {
    onChange({
      ...draft,
      files: draft.files.map((file, i) => (i === index ? next : file)),
    });
  }
  return (
    <>
      <p className="field-help">{t("emberEditor.filesHelp")}</p>
      <AllocationList
        items={draft.files}
        rowLabel={(index) =>
          t("emberEditor.item", {
            name: t("emberEditor.configFile"),
            number: index + 1,
          })
        }
        removeLabel={(index) =>
          t("emberEditor.removeItem", {
            name: t("emberEditor.configFile"),
            number: index + 1,
          })
        }
        addLabel={t("emberEditor.add", { name: t("emberEditor.configFile") })}
        onRemove={(index) =>
          onChange({
            ...draft,
            files: draft.files.filter((_, i) => i !== index),
          })
        }
        onAdd={() =>
          onChange({
            ...draft,
            files: [
              ...draft.files,
              {
                file: {
                  path: "",
                  parser: ConfigParser.Properties,
                  values: {},
                  create: true,
                },
                values: [],
              },
            ],
          })
        }
        renderBody={(item, index) => (
          <>
            <EditorCheck
              label={t("emberEditor.createFile")}
              value={item.file.create}
              onChange={(create) =>
                change(index, { ...item, file: { ...item.file, create } })
              }
            />
            <ReplacementValues
              rows={item.values}
              onChange={(values) => change(index, { ...item, values })}
            />
          </>
        )}
      >
        {(item, index) => (
          <>
            <EditorText
              label={t("emberEditor.filePath")}
              value={item.file.path}
              required
              onChange={(path) =>
                change(index, { ...item, file: { ...item.file, path } })
              }
            />
            <FormField label={t("emberEditor.parser")}>
              <Select
                value={item.file.parser}
                onValueChange={(parser: EmberConfigFile["parser"]) =>
                  change(index, { ...item, file: { ...item.file, parser } })
                }
              >
                <option value={ConfigParser.Properties}>Properties</option>
                <option value={ConfigParser.JSON}>JSON</option>
                <option value={ConfigParser.YAML}>YAML</option>
                <option value={ConfigParser.INI}>INI</option>
                <option value={ConfigParser.File}>
                  {t("emberEditor.textFile")}
                </option>
              </Select>
            </FormField>
          </>
        )}
      </AllocationList>
    </>
  );
}
