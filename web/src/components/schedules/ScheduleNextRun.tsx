import { EMPTY_VALUE } from "../../utils/formatDisplayValue";
import "./schedule-next-run.css";

export function ScheduleNextRun({
  at,
  enabled,
}: {
  at: number;
  enabled: boolean;
}) {
  if (at <= 0) return <>{EMPTY_VALUE}</>;
  const date = new Date(at * 1000);
  return (
    <span className="schedule-next-run">
      <time
        dateTime={date.toISOString()}
        className={enabled ? undefined : "schedule-next-run-hidden"}
        aria-hidden={!enabled}
      >
        {date.toLocaleString()}
      </time>
      {!enabled && <span>{EMPTY_VALUE}</span>}
    </span>
  );
}
