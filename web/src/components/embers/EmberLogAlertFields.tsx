import { useTranslation } from "react-i18next";
import {
  LogAction,
  LogPhase,
  LogSeverity,
  type LogAlert,
} from "../../contracts/logAlerts";
import { AllocationList } from "../forms/AllocationList";
import { FormField } from "../forms/FormField";
import { Select } from "../forms/Select";
import { Textarea } from "../forms/Textarea";
import { EditorCheck, EditorGroup, EditorText } from "./EmberEditorFields";
import { optionalValue, type EmberDraft } from "./emberDraft";

export function EmberLogAlertFields({
  draft,
  onChange,
}: {
  draft: EmberDraft;
  onChange: (draft: EmberDraft) => void;
}) {
  const { t } = useTranslation();
  const alerts = draft.definition.log_alerts ?? [];
  function save(next: LogAlert[]) {
    onChange({
      ...draft,
      definition: { ...draft.definition, log_alerts: next },
    });
  }
  function change(index: number, patch: Partial<LogAlert>) {
    save(
      alerts.map((alert, i) => (i === index ? { ...alert, ...patch } : alert)),
    );
  }
  return (
    <EditorGroup
      title={t("logAlerts.editorTitle")}
      description={t("logAlerts.editorHelp")}
    >
      <AllocationList
        items={alerts}
        maxItems={32}
        rowLabel={(index) => t("logAlerts.rule", { number: index + 1 })}
        removeLabel={(index) =>
          t("logAlerts.removeRule", { number: index + 1 })
        }
        addLabel={t("logAlerts.addRule")}
        onRemove={(index) => save(alerts.filter((_, i) => i !== index))}
        onAdd={() =>
          save([
            ...alerts,
            {
              title: "",
              message: "",
              severity: LogSeverity.Error,
              any: [{ value: "" }],
            },
          ])
        }
        renderBody={(alert, index) => (
          <>
            <FormField label={t("logAlerts.guidance")}>
              <Textarea
                rows={2}
                value={alert.message}
                maxLength={2048}
                required
                onChange={(event) =>
                  change(index, { message: event.target.value })
                }
              />
            </FormField>
            <div className="ember-editor-grid">
              <FormField label={t("logAlerts.phase")}>
                <Select
                  value={alert.phase ?? ""}
                  onValueChange={(phase) =>
                    save(
                      alerts.map((item, i) =>
                        i === index
                          ? optionalValue(item, "phase", phase || undefined)
                          : item,
                      ),
                    )
                  }
                >
                  <option value="">{t("logAlerts.bothPhases")}</option>
                  <option value={LogPhase.Runtime}>
                    {t("emberEditor.runtime")}
                  </option>
                  <option value={LogPhase.Installation}>
                    {t("emberEditor.installation")}
                  </option>
                </Select>
              </FormField>
              <FormField label={t("logAlerts.action")}>
                <Select
                  value={alert.action?.type ?? ""}
                  onValueChange={(type) =>
                    save(
                      alerts.map((item, i) =>
                        i === index
                          ? optionalValue(
                              item,
                              "action",
                              type
                                ? {
                                    type,
                                    ...(type === LogAction.OpenFile
                                      ? { path: "" }
                                      : {}),
                                  }
                                : undefined,
                            )
                          : item,
                      ),
                    )
                  }
                >
                  <option value="">{t("logAlerts.noAction")}</option>
                  <option value={LogAction.OpenFile}>
                    {t("logAlerts.openFileAction")}
                  </option>
                  <option value={LogAction.ServerSettings}>
                    {t("logAlerts.settingsAction")}
                  </option>
                </Select>
              </FormField>
            </div>
            {alert.action?.type === LogAction.OpenFile && (
              <EditorText
                label={t("emberEditor.filePath")}
                value={alert.action.path ?? ""}
                required
                maxLength={512}
                onChange={(path) =>
                  change(index, { action: { type: LogAction.OpenFile, path } })
                }
              />
            )}
            <AllocationList
              items={alert.any}
              maxItems={16}
              rowLabel={(condition) =>
                t("logAlerts.condition", { number: condition + 1 })
              }
              removeLabel={(condition) =>
                t("logAlerts.removeCondition", { number: condition + 1 })
              }
              addLabel={t("logAlerts.addCondition")}
              onRemove={(condition) =>
                change(index, {
                  any: alert.any.filter((_, i) => i !== condition),
                })
              }
              onAdd={() =>
                change(index, { any: [...alert.any, { value: "" }] })
              }
              renderBody={(match, condition) => (
                <div className="ember-editor-checks">
                  <EditorCheck
                    label={t("emberEditor.regularExpression")}
                    value={match.regex}
                    onChange={(regex) =>
                      change(index, {
                        any: alert.any.map((item, i) =>
                          i === condition ? { ...item, regex } : item,
                        ),
                      })
                    }
                  />
                  <EditorCheck
                    label={t("logAlerts.ignoreCase")}
                    value={match.ignore_case}
                    onChange={(ignore_case) =>
                      change(index, {
                        any: alert.any.map((item, i) =>
                          i === condition ? { ...item, ignore_case } : item,
                        ),
                      })
                    }
                  />
                </div>
              )}
            >
              {(match, condition) => (
                <EditorText
                  label={t("emberEditor.matchValue")}
                  value={match.value}
                  required
                  maxLength={512}
                  onChange={(value) =>
                    change(index, {
                      any: alert.any.map((item, i) =>
                        i === condition ? { ...item, value } : item,
                      ),
                    })
                  }
                />
              )}
            </AllocationList>
          </>
        )}
      >
        {(alert, index) => (
          <>
            <EditorText
              label={t("logAlerts.title")}
              value={alert.title}
              required
              maxLength={120}
              onChange={(title) => change(index, { title })}
            />
            <FormField label={t("logAlerts.severity")}>
              <Select
                value={alert.severity}
                onValueChange={(severity) => change(index, { severity })}
              >
                <option value={LogSeverity.Error}>
                  {t("logAlerts.error")}
                </option>
                <option value={LogSeverity.Warning}>
                  {t("logAlerts.warning")}
                </option>
              </Select>
            </FormField>
          </>
        )}
      </AllocationList>
    </EditorGroup>
  );
}
