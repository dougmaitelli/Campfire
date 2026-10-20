import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import { Fieldset } from "../forms/Fieldset";
import { FormField } from "../forms/FormField";
import { TextInput } from "../forms/TextInput";
import { Button } from "../forms/Button";
import { InlineMessage } from "../status/InlineMessage";
import { useEffect, useId, useState } from "react";
import { previewSchedule } from "../../api/activity";
import "./cron-field.css";

const presets = [
  {
    get label() {
      return i18n.t("cronField.every15Minutes");
    },
    value: "*/15 * * * *",
    get description() {
      return i18n.t("cronField.every15Minutes");
    },
  },
  {
    get label() {
      return i18n.t("cronField.hourly");
    },
    value: "0 * * * *",
    get description() {
      return i18n.t("cronField.everyHourOnTheHour");
    },
  },
  {
    get label() {
      return i18n.t("cronField.daily");
    },
    value: "0 3 * * *",
    get description() {
      return i18n.t("cronField.everyDayAt0300");
    },
  },
  {
    get label() {
      return i18n.t("cronField.weekdays");
    },
    value: "0 3 * * 1-5",
    get description() {
      return i18n.t("cronField.mondayThroughFridayAt0300");
    },
  },
  {
    get label() {
      return i18n.t("cronField.weekly");
    },
    value: "0 3 * * 0",
    get description() {
      return i18n.t("cronField.everySundayAt0300");
    },
  },
  {
    get label() {
      return i18n.t("cronField.monthly");
    },
    value: "0 3 1 * *",
    get description() {
      return i18n.t("cronField.onTheFirstDayOfEachMonthAt0300");
    },
  },
];
const fields = [
  i18n.t("cronField.minute"),
  i18n.t("cronField.hour"),
  i18n.t("cronField.day"),
  i18n.t("cronField.month"),
  i18n.t("cronField.weekday"),
];

export function CronField({
  expression,
  timeZone,
  onChange,
  onTimeZoneChange,
  onValidated,
  disabled,
}: {
  expression: string;
  timeZone: string;
  onChange: (value: string) => void;
  onTimeZoneChange: (value: string) => void;
  onValidated: (key: string) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const id = useId();
  const normalized = expression.trim().replace(/\s+/g, " ");
  const key = `${normalized}\n${timeZone}`;
  const [preview, setPreview] = useState<{
    key: string;
    next: number[];
    error: string | null;
  } | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    const timer = setTimeout(() => {
      void previewSchedule(
        normalized,
        timeZone,
        AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
      )
        .then((next) => {
          if (!controller.signal.aborted) {
            setPreview({ key, next, error: null });
            onValidated(key);
          }
        })
        .catch((cause: unknown) => {
          if (!controller.signal.aborted)
            setPreview({
              key,
              next: [],
              error:
                cause instanceof Error
                  ? cause.message
                  : i18n.t("cronField.unableToPreviewSchedule"),
            });
        });
    }, 300);
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [normalized, timeZone, key, onValidated]);
  const current = preview?.key === key ? preview : null;
  const tokens = normalized ? normalized.split(" ") : [];
  const zones = Array.from(
    new Set([
      "UTC",
      Intl.DateTimeFormat().resolvedOptions().timeZone,
      ...Intl.supportedValuesOf("timeZone"),
    ]),
  );
  function format(timestamp: number) {
    try {
      return new Intl.DateTimeFormat(undefined, {
        timeZone,
        weekday: "short",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        timeZoneName: "short",
      }).format(new Date(timestamp * 1000));
    } catch {
      return new Date(timestamp * 1000).toISOString();
    }
  }
  return (
    <Fieldset className="cron-field" disabled={disabled}>
      <FormField label={t("cronField.cronSchedule")}>
        <TextInput
          id={`${id}-expression`}
          className="cron-expression"
          value={expression}
          maxLength={256}
          autoComplete="off"
          spellCheck={false}
          aria-describedby={`${id}-help ${id}-preview`}
          aria-invalid={!!current?.error}
          onChange={(event) => onChange(event.target.value)}
        />
      </FormField>
      <dl className="cron-fields" aria-label={t("cronField.cronFields")}>
        {fields.map((field, index) => (
          <div key={field}>
            <dt>{field}</dt>
            <dd>{tokens[index] ?? "—"}</dd>
          </div>
        ))}
      </dl>
      <div
        className="cron-presets"
        aria-label={t("cronField.fillCronSchedule")}
      >
        {presets.map((preset) => (
          <Button
            size="small"
            type="button"

            key={preset.value}
            aria-pressed={normalized === preset.value}
            onClick={() => onChange(preset.value)}
          >
            {preset.label}
          </Button>
        ))}
      </div>
      <p id={`${id}-help`} className="field-help">
        {t("cronField.useForAnyValueCommasForListsForRangesAnd")}
      </p>
      <FormField label={t("cronField.timeZone")}>
        <TextInput
          id={`${id}-zone`}
          list={`${id}-zones`}
          value={timeZone}
          autoComplete="off"
          maxLength={100}
          onChange={(event) => onTimeZoneChange(event.target.value)}
        />
      </FormField>
      <datalist id={`${id}-zones`}>
        {zones.map((zone) => (
          <option key={zone} value={zone} />
        ))}
      </datalist>
      <div id={`${id}-preview`} className="cron-preview" aria-live="polite">
        <p>
          {presets.find((preset) => preset.value === normalized)?.description ??
            t("cronField.customCronSchedule")}
        </p>
        {current?.error ? (
          <InlineMessage role="alert">{current.error}</InlineMessage>
        ) : current ? (
          <>
            <span className="field-help">{t("cronField.nextRuns")}</span>
            <ul>
              {current.next.map((timestamp) => (
                <li key={timestamp}>
                  <time dateTime={new Date(timestamp * 1000).toISOString()}>
                    {format(timestamp)}
                  </time>
                </li>
              ))}
            </ul>
          </>
        ) : (
          <InlineMessage tone="info" role="status">
            {t("cronField.checkingSchedule")}
          </InlineMessage>
        )}
      </div>
    </Fieldset>
  );
}
