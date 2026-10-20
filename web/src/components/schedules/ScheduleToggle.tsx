import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { setScheduleEnabled, type Schedule } from "../../api/activity";
import { ToggleSwitch } from "../forms/ToggleSwitch";

export function ScheduleToggle({
  schedule,
  disabled = false,
  onSaved,
  onError,
}: {
  schedule: Schedule;
  disabled?: boolean;
  onSaved: (schedule: Schedule) => void;
  onError: (message: string | null) => void;
}) {
  const { t } = useTranslation();
  const [busy, setBusy] = useState(false);
  const pending = useRef(false);
  async function toggle(enabled: boolean) {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    onError(null);
    try {
      onSaved(await setScheduleEnabled(schedule.id, enabled));
    } catch (cause) {
      onError(
        cause instanceof Error
          ? cause.message
          : t("scheduleToggle.unableToSave"),
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <ToggleSwitch
      aria-label={t("scheduleToggle.enabled", { name: schedule.name })}
      aria-busy={busy}
      checked={schedule.enabled}
      disabled={disabled || busy}
      onChange={(event) => void toggle(event.target.checked)}
    />
  );
}
