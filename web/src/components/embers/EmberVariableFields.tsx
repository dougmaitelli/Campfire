import { useTranslation } from "react-i18next";
import type { EmberVariable, EmberVariableValidation } from "../../api/embers";
import { VariableType, VariableFormat } from "../../contracts/domains";
import { FormField } from "../forms/FormField";
import { Select } from "../forms/Select";
import { TextInput } from "../forms/TextInput";
import { AllocationList } from "../forms/AllocationList";
import { EditorCheck, EditorText, AllowedValues } from "./EmberEditorFields";
import type { EmberDraft } from "./emberDraft";
import { optionalValue } from "./emberDraft";

export function EmberVariableFields({
  draft,
  onChange,
}: {
  draft: EmberDraft;
  onChange: (value: EmberDraft) => void;
}) {
  const { t } = useTranslation();
  const variables = draft.definition.variables ?? [];
  function save(next: EmberVariable[]) {
    onChange({
      ...draft,
      definition: { ...draft.definition, variables: next },
    });
  }
  function change(index: number, variable: EmberVariable) {
    save(variables.map((item, i) => (i === index ? variable : item)));
  }
  return (
    <>
      <p className="field-help">{t("emberEditor.variablesHelp")}</p>
      <AllocationList
        className="ember-variable-list"
        items={variables}
        maxItems={128}
        rowLabel={(index) => {
          const variable = variables[index];
          return variable?.name.trim().length
            ? variable.name
            : t("emberEditor.item", {
                name: t("emberEditor.variable"),
                number: index + 1,
              });
        }}
        removeLabel={(index) =>
          t("emberEditor.removeItem", {
            name: t("emberEditor.variable"),
            number: index + 1,
          })
        }
        addLabel={t("emberEditor.add", { name: t("emberEditor.variable") })}
        onRemove={(index) => save(variables.filter((_, i) => i !== index))}
        onAdd={() =>
          save([
            ...variables,
            { name: "", default: "", required: false, editable: true },
          ])
        }
        renderBody={(variable, index) => {
          const rules =
            variable.validation ??
            (variable.pattern ? { type: VariableType.String } : undefined);
          function rule(next: Partial<EmberVariableValidation>) {
            change(index, {
              ...variable,
              validation: { type: VariableType.String, ...rules, ...next },
            });
          }
          return (
            <>
              <div className="ember-editor-checks">
                <EditorCheck
                  label={t("emberEditor.required")}
                  value={variable.required}
                  onChange={(required) =>
                    change(index, { ...variable, required })
                  }
                />
                <EditorCheck
                  label={t("emberEditor.readOnly")}
                  value={!variable.editable}
                  onChange={(readOnly) =>
                    change(index, { ...variable, editable: !readOnly })
                  }
                />
                <EditorCheck
                  label={t("emberEditor.hidden")}
                  value={variable.hidden}
                  onChange={(hidden) => change(index, { ...variable, hidden })}
                />
              </div>
              <EditorCheck
                label={t("emberEditor.useValidation")}
                value={!!rules}
                onChange={(enabled) =>
                  change(
                    index,
                    optionalValue(
                      enabled
                        ? variable
                        : optionalValue(variable, "pattern", undefined),
                      "validation",
                      enabled ? { type: VariableType.String } : undefined,
                    ),
                  )
                }
              />
              {rules && (
                <>
                  <div className="ember-editor-grid">
                    <FormField label={t("emberEditor.valueType")}>
                      <Select
                        value={rules.format ?? rules.type}
                        onValueChange={(selection) => {
                          const type =
                            selection === VariableFormat.URL
                              ? VariableType.String
                              : selection;
                          const next: EmberVariableValidation =
                            type === rules.type
                              ? { ...rules }
                              : {
                                  type,
                                  nullable: rules.nullable ?? false,
                                  ...(rules.choices
                                    ? { choices: rules.choices }
                                    : {}),
                                };
                          if (selection === VariableFormat.URL)
                            next.format = VariableFormat.URL;
                          else delete next.format;
                          change(index, { ...variable, validation: next });
                        }}
                      >
                        {Object.values(VariableType).map((type) => (
                          <option key={type} value={type}>
                            {t(`emberEditor.types.${type}`)}
                          </option>
                        ))}
                        <option value={VariableFormat.URL}>
                          {t("emberEditor.types.url")}
                        </option>
                      </Select>
                    </FormField>
                    <EditorText
                      label={t("emberEditor.pattern")}
                      value={variable.pattern ?? ""}
                      onChange={(pattern) =>
                        change(index, { ...variable, pattern })
                      }
                    />
                  </div>
                  <EditorCheck
                    label={t("emberEditor.nullable")}
                    value={rules.nullable}
                    onChange={(nullable) => rule({ nullable })}
                  />
                  {rules.type === VariableType.String && (
                    <>
                      <div className="ember-editor-grid">
                        {(["min_length", "max_length"] as const).map((key) => (
                          <FormField key={key} label={t(`emberEditor.${key}`)}>
                            <TextInput
                              type="number"
                              min={0}
                              max={8192}
                              step={1}
                              value={rules[key] ?? ""}
                              onChange={(event) =>
                                change(index, {
                                  ...variable,
                                  validation: optionalValue(
                                    rules,
                                    key,
                                    event.target.value === ""
                                      ? undefined
                                      : Number(event.target.value),
                                  ),
                                })
                              }
                            />
                          </FormField>
                        ))}
                      </div>
                    </>
                  )}
                  {(rules.type === VariableType.Integer ||
                    rules.type === VariableType.Numeric) && (
                    <div className="ember-editor-grid">
                      {(["min", "max"] as const).map((key) => (
                        <FormField key={key} label={t(`emberEditor.${key}`)}>
                          <TextInput
                            type="number"
                            step={
                              rules.type === VariableType.Integer ? 1 : "any"
                            }
                            value={rules[key] ?? ""}
                            onChange={(event) =>
                              change(index, {
                                ...variable,
                                validation: optionalValue(
                                  rules,
                                  key,
                                  event.target.value === ""
                                    ? undefined
                                    : Number(event.target.value),
                                ),
                              })
                            }
                          />
                        </FormField>
                      ))}
                    </div>
                  )}
                  <AllowedValues
                    values={rules.choices ?? []}
                    onChange={(choices) => rule({ choices })}
                  />
                </>
              )}
            </>
          );
        }}
      >
        {(variable, index) => (
          <>
            <EditorText
              label={t("emberEditor.variableName")}
              value={variable.name}
              required
              pattern="[A-Z][A-Z0-9_]*"
              onChange={(name) => change(index, { ...variable, name })}
            />
            <EditorText
              label={t("emberEditor.defaultValue")}
              value={variable.default}
              onChange={(next) => change(index, { ...variable, default: next })}
            />
          </>
        )}
      </AllocationList>
    </>
  );
}
