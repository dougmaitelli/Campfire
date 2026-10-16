import { useEffect, useId, useState } from "react";
import { previewSchedule } from "../../api/activity";
import "./cron-field.css";

const presets = [
  {
    label: "Every 15 minutes",
    value: "*/15 * * * *",
    description: "Every 15 minutes",
  },
  {
    label: "Hourly",
    value: "0 * * * *",
    description: "Every hour, on the hour",
  },
  { label: "Daily", value: "0 3 * * *", description: "Every day at 03:00" },
  {
    label: "Weekdays",
    value: "0 3 * * 1-5",
    description: "Monday through Friday at 03:00",
  },
  { label: "Weekly", value: "0 3 * * 0", description: "Every Sunday at 03:00" },
  {
    label: "Monthly",
    value: "0 3 1 * *",
    description: "On the first day of each month at 03:00",
  },
];
const fields = ["Minute", "Hour", "Day", "Month", "Weekday"];

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
                  : "Unable to preview schedule.",
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
    <fieldset className="cron-field" disabled={disabled}>
      <label htmlFor={`${id}-expression`}>Cron schedule</label>
      <input
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
      <dl className="cron-fields" aria-label="Cron fields">
        {fields.map((field, index) => (
          <div key={field}>
            <dt>{field}</dt>
            <dd>{tokens[index] ?? "—"}</dd>
          </div>
        ))}
      </dl>
      <div className="cron-presets" aria-label="Fill cron schedule">
        {presets.map((preset) => (
          <button
            type="button"
            className="button"
            key={preset.value}
            aria-pressed={normalized === preset.value}
            onClick={() => onChange(preset.value)}
          >
            {preset.label}
          </button>
        ))}
      </div>
      <p id={`${id}-help`} className="field-help">
        Use * for any value, commas for lists, - for ranges, and / for steps.
        Sunday is 0. Day and weekday restrictions match either field.
      </p>
      <label htmlFor={`${id}-zone`}>Time zone</label>
      <input
        id={`${id}-zone`}
        list={`${id}-zones`}
        value={timeZone}
        autoComplete="off"
        maxLength={100}
        onChange={(event) => onTimeZoneChange(event.target.value)}
      />
      <datalist id={`${id}-zones`}>
        {zones.map((zone) => (
          <option key={zone} value={zone} />
        ))}
      </datalist>
      <div id={`${id}-preview`} className="cron-preview" aria-live="polite">
        <p>
          {presets.find((preset) => preset.value === normalized)?.description ??
            "Custom cron schedule"}
        </p>
        {current?.error ? (
          <p className="error-message" role="alert">
            {current.error}
          </p>
        ) : current ? (
          <>
            <span className="field-help">Next runs</span>
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
          <p className="field-help" role="status">
            Checking schedule…
          </p>
        )}
      </div>
    </fieldset>
  );
}
