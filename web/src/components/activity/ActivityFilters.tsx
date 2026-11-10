import { useState, useRef, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import type { ActivityChoices, ActivityFilter } from "../../api/serverActivity";
import { MAX_ACTIVITY_TIME } from "../../api/serverActivity";
import { gameEventFieldLabel } from "../../api/gameEvents";
import { Button } from "../forms/Button";
import { FormField } from "../forms/FormField";
import { Select } from "../forms/Select";
import { TextInput } from "../forms/TextInput";
import { DateTimeInput } from "../forms/DateTimeInput";
import { ListToolbar } from "../layout/ListToolbar";
import { InlineMessage } from "../status/InlineMessage";
import { NavigationIcon } from "../navigation/NavigationIcon";
import { GameLimits } from "../../config/policies";

function localTime(value: number | undefined): string {
  if (
    !value ||
    !Number.isSafeInteger(value) ||
    value < 0 ||
    value > MAX_ACTIVITY_TIME
  )
    return "";
  const date = new Date(value * 1000);
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
    .toISOString()
    .slice(0, 16);
}
const statusOptions = {
  "": [],
  game: [],
  operations: [
    "pending",
    "running",
    "failed",
    "completed",
    "cancelled",
    "cancelling",
    "dispatching",
    "succeeded",
    "interrupted",
    "skipped",
    "dispatch_failed",
  ],
  alert: ["active", "dismissed", "archived", "raised", "recovered"],
} as const;

export function ActivityFilters({
  filter,
  choices,
  onApply,
}: {
  filter: ActivityFilter;
  choices: ActivityChoices | undefined;
  onApply: (filter: ActivityFilter) => void;
}) {
  const { t } = useTranslation();
  const kind = filter.kind ?? "";
  const [search, setSearch] = useState(filter.search ?? "");
  const [state, setState] = useState(filter.state ?? "");
  const [type, setType] = useState(filter.type ?? "");
  const [from, setFrom] = useState(localTime(filter.from));
  const [to, setTo] = useState(localTime(filter.to));
  const [fields, setFields] = useState(() =>
    Object.entries(filter.fields ?? {}).map(([field, value], id) => ({
      id,
      field,
      value,
    })),
  );
  const next = useRef(fields.length);
  const [error, setError] = useState("");
  function apply(event: FormEvent) {
    event.preventDefault();
    const start = from ? new Date(from).getTime() / 1000 : undefined;
    const end = to ? new Date(to).getTime() / 1000 : undefined;
    if (
      (start !== undefined &&
        (!Number.isSafeInteger(start) ||
          start < 1 ||
          start > MAX_ACTIVITY_TIME)) ||
      (end !== undefined &&
        (!Number.isSafeInteger(end) || end < 1 || end > MAX_ACTIVITY_TIME)) ||
      (start !== undefined && end !== undefined && start >= end)
    ) {
      setError(t("gameEvents.invalidRange"));
      return;
    }
    const data: Record<string, string> = {};
    for (const field of fields) {
      if (!field.field && !field.value) continue;
      if (!field.field || Object.hasOwn(data, field.field)) {
        setError(t("gameEvents.invalidFields"));
        return;
      }
      data[field.field] = field.value;
    }
    setError("");
    onApply({
      kind,
      search,
      ...(start !== undefined ? { from: start } : {}),
      ...(end !== undefined ? { to: end } : {}),
      ...(kind === "game" ? { type, fields: data } : {}),
      ...(kind !== "game" && kind !== "" ? { state } : {}),
    });
  }
  return (
    <form onSubmit={apply}>
      {error && <InlineMessage role="alert">{error}</InlineMessage>}
      <ListToolbar>
        <FormField label={t("serverActivity.type")}>
          <Select
            value={kind}
            onValueChange={(value) =>
              onApply({
                kind: value,
                ...(filter.search ? { search: filter.search } : {}),
                ...(filter.from ? { from: filter.from } : {}),
                ...(filter.to ? { to: filter.to } : {}),
              })
            }
          >
            {(["", "game", "operations", "alert"] as const).map((value) => (
              <option key={value} value={value}>
                {t(`serverActivity.filters.${value || "all"}`)}
              </option>
            ))}
          </Select>
        </FormField>
        <FormField label={t("gameEvents.search")}>
          <TextInput
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            maxLength={1024}
          />
        </FormField>
        <FormField label={t("serverActivity.from")}>
          <DateTimeInput
            value={from}
            onChange={(event) => setFrom(event.target.value)}
          />
        </FormField>
        <FormField label={t("serverActivity.to")}>
          <DateTimeInput
            value={to}
            onChange={(event) => setTo(event.target.value)}
          />
        </FormField>
        {kind && kind !== "game" && (
          <FormField label={t("common.status")}>
            <Select value={state} onValueChange={setState}>
              <option value="">{t("serverActivity.anyStatus")}</option>
              {statusOptions[kind].map((value) => (
                <option key={value} value={value}>
                  {t(`serverActivity.states.${value}`)}
                </option>
              ))}
            </Select>
          </FormField>
        )}
        {kind === "game" && (
          <>
            <FormField label={t("gameEvents.type")}>
              <Select value={type} onValueChange={setType} disabled={!choices}>
                <option value="">{t("gameEvents.allTypes")}</option>
                {[
                  ...new Set([
                    ...(choices?.game.types ?? []),
                    ...(type ? [type] : []),
                  ]),
                ].map((value) => (
                  <option key={value} value={value}>
                    {value}
                  </option>
                ))}
              </Select>
            </FormField>
            {fields.map((field, index) => (
              <div className="activity-field-filter" key={field.id}>
                <FormField label={t("gameEvents.field")}>
                  <Select
                    value={field.field}
                    disabled={!choices}
                    onValueChange={(value) =>
                      setFields((items) =>
                        items.map((item) =>
                          item.id === field.id
                            ? { ...item, field: value, value: "" }
                            : item,
                        ),
                      )
                    }
                  >
                    <option value="">{t("gameEvents.selectField")}</option>
                    {[
                      ...new Set([
                        ...Object.keys(choices?.game.fields ?? {}),
                        ...(field.field ? [field.field] : []),
                      ]),
                    ].map((key) => (
                      <option
                        key={key}
                        value={key}
                        disabled={fields.some(
                          (item) => item.id !== field.id && item.field === key,
                        )}
                      >
                        {gameEventFieldLabel(choices?.game, type, key)}
                      </option>
                    ))}
                  </Select>
                </FormField>
                <FormField label={t("gameEvents.value")}>
                  <Select
                    value={field.value}
                    disabled={!choices || !field.field}
                    onValueChange={(value) =>
                      setFields((items) =>
                        items.map((item) =>
                          item.id === field.id ? { ...item, value } : item,
                        ),
                      )
                    }
                  >
                    <option value="">{t("gameEvents.allValues")}</option>
                    {[
                      ...new Set([
                        ...(choices?.game.fields[field.field] ?? []),
                        ...(field.value ? [field.value] : []),
                      ]),
                    ].map((value) => (
                      <option key={value} value={value}>
                        {value}
                      </option>
                    ))}
                  </Select>
                </FormField>
                <Button
                  type="button"
                  iconOnly
                  aria-label={t("gameEvents.removeFieldFilter", {
                    index: index + 1,
                  })}
                  onClick={() =>
                    setFields((items) =>
                      items.filter((item) => item.id !== field.id),
                    )
                  }
                >
                  <NavigationIcon name="delete" />
                </Button>
              </div>
            ))}
            <Button
              type="button"
              disabled={!choices || fields.length >= GameLimits.Fields}
              onClick={() =>
                setFields((items) => [
                  ...items,
                  { id: next.current++, field: "", value: "" },
                ])
              }
            >
              {t("gameEvents.addFieldFilter")}
            </Button>
          </>
        )}
        <Button type="submit">{t("gameEvents.apply")}</Button>
        <Button
          type="button"
          onClick={() => {
            setSearch("");
            setState("");
            setType("");
            setFrom("");
            setTo("");
            setFields([]);
            setError("");
            onApply({ kind });
          }}
        >
          {t("gameEvents.clear")}
        </Button>
      </ListToolbar>
    </form>
  );
}
