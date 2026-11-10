import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  readGameEventFilterOptions,
  type GameEventFilterOptions,
} from "../../api/gameEvents";
import type { ServerIntegrationInput } from "../../api/integrations";
import { GameLimits } from "../../config/policies";
import { EditableList } from "../forms/EditableList";
import { FormField } from "../forms/FormField";
import { Select } from "../forms/Select";
import { InlineMessage } from "../status/InlineMessage";
import "./game-event-subscription-editor.css";

type Subscription = ServerIntegrationInput["subscriptions"][number];
type Field = { name: string; value: string };
export type GameEventRuleDraft = { type: string | null; fields: Field[] };

export function GameEventSubscriptionEditor({
  serverID,
  value,
  rules,
  onChange,
}: {
  serverID: string;
  value: Subscription;
  rules: GameEventRuleDraft[];
  onChange: (value: Subscription, rules: GameEventRuleDraft[]) => void;
}) {
  const { t } = useTranslation();
  const [options, setOptions] = useState<GameEventFilterOptions | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    const controller = new AbortController();
    void readGameEventFilterOptions(serverID, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setOptions(result);
      })
      .catch((cause: unknown) => {
        if (!controller.signal.aborted)
          setError(
            cause instanceof Error ? cause.message : t("integrations.failed"),
          );
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [serverID, t]);

  function update(next: GameEventRuleDraft[]) {
    const entries = next
      .filter(
        (rule): rule is GameEventRuleDraft & { type: string } =>
          rule.type !== null,
      )
      .map((rule) => ({
        type: rule.type,
        fields: Object.fromEntries(
          rule.fields
            .filter((field) => field.name)
            .map((field) => [field.name, field.value]),
        ),
      }));
    onChange(
      {
        category: value.category,
        ...(entries.length ? { rules: entries } : {}),
      },
      next,
    );
  }
  function replace(index: number, rule: GameEventRuleDraft) {
    update(rules.map((item, at) => (at === index ? rule : item)));
  }

  return (
    <div className="integration-event-filters">
      <p className="field-help">{t("serverIntegrations.gameFilterHelp")}</p>
      {error && <InlineMessage tone="warning">{error}</InlineMessage>}
      <EditableList
        items={rules}
        rowLabel={(index) =>
          t("serverIntegrations.eventTypeRow", { index: index + 1 })
        }
        removeLabel={(index) =>
          t("serverIntegrations.removeType", { index: index + 1 })
        }
        addLabel={t("serverIntegrations.addType")}
        addDisabled={loading || rules.some((rule) => rule.type === null)}
        onAdd={() => update([...rules, { type: null, fields: [] }])}
        onRemove={(index) => update(rules.filter((_, at) => at !== index))}
        renderBody={(rule, index) => (
          <EditableList
            items={rule.fields}
            rowLabel={(at) =>
              t("serverIntegrations.eventFieldRow", { index: at + 1 })
            }
            removeLabel={(at) =>
              t("gameEvents.removeFieldFilter", { index: at + 1 })
            }
            addLabel={t("gameEvents.addFieldFilter")}
            addDisabled={loading || rule.fields.some((field) => !field.name)}
            maxItems={GameLimits.Fields}
            onAdd={() =>
              update(
                rules.map((item, at) =>
                  at === index
                    ? {
                        ...item,
                        fields: [...item.fields, { name: "", value: "" }],
                      }
                    : item,
                ),
              )
            }
            onRemove={(at) =>
              replace(index, {
                ...rule,
                fields: rule.fields.filter(
                  (_, fieldIndex) => fieldIndex !== at,
                ),
              })
            }
          >
            {(field, fieldIndex) => (
              <>
                <FormField label={t("gameEvents.field")}>
                  <Select
                    value={field.name}
                    disabled={loading}
                    onValueChange={(name) =>
                      replace(index, {
                        ...rule,
                        fields: rule.fields.map((item, at) =>
                          at === fieldIndex ? { name, value: "" } : item,
                        ),
                      })
                    }
                  >
                    <option value="">{t("gameEvents.selectField")}</option>
                    {[
                      ...new Set([
                        ...Object.keys(options?.fields ?? {}),
                        ...rule.fields.map((item) => item.name).filter(Boolean),
                      ]),
                    ]
                      .sort()
                      .map((name) => (
                        <option
                          key={name}
                          value={name}
                          disabled={rule.fields.some(
                            (item, at) =>
                              at !== fieldIndex && item.name === name,
                          )}
                        >
                          {name}
                        </option>
                      ))}
                  </Select>
                </FormField>
                <FormField label={t("gameEvents.value")}>
                  <Select
                    value={field.value}
                    disabled={loading || !field.name}
                    onValueChange={(next) =>
                      replace(index, {
                        ...rule,
                        fields: rule.fields.map((item, at) =>
                          at === fieldIndex ? { ...item, value: next } : item,
                        ),
                      })
                    }
                  >
                    <option value="">{t("gameEvents.allValues")}</option>
                    {[
                      ...new Set([
                        ...(options?.fields[field.name] ?? []),
                        ...(field.value ? [field.value] : []),
                      ]),
                    ].map((item) => (
                      <option key={item} value={item}>
                        {item}
                      </option>
                    ))}
                  </Select>
                </FormField>
              </>
            )}
          </EditableList>
        )}
      >
        {(rule, index) => (
          <FormField label={t("gameEvents.type")}>
            <Select
              value={rule.type ?? "__pending__"}
              disabled={loading}
              onValueChange={(type) => replace(index, { ...rule, type })}
            >
              <option value="__pending__" disabled>
                {t("serverIntegrations.chooseType")}
              </option>
              <option value="">{t("gameEvents.allTypes")}</option>
              {[
                ...new Set([
                  ...(options?.types ?? []),
                  ...rules
                    .map((item) => item.type)
                    .filter((type): type is string => !!type),
                ]),
              ]
                .sort()
                .map((kind) => (
                  <option key={kind} value={kind}>
                    {kind}
                  </option>
                ))}
            </Select>
          </FormField>
        )}
      </EditableList>
    </div>
  );
}
