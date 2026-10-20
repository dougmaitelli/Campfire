import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import { StatusPill, type StatusTone } from "../status/StatusPill";

const states: Record<string, { label: string; tone: StatusTone }> = {
  available: {
    get label() {
      return i18n.t("backupState.available");
    },
    tone: "positive",
  },
  syncing: {
    get label() {
      return i18n.t("backupState.syncing");
    },
    tone: "pending",
  },
  deleting: {
    get label() {
      return i18n.t("backupState.deleting");
    },
    tone: "pending",
  },
  failed: {
    get label() {
      return i18n.t("common.failed");
    },
    tone: "negative",
  },
  error: {
    get label() {
      return i18n.t("backupState.error");
    },
    tone: "negative",
  },
  unavailable: {
    get label() {
      return i18n.t("common.unavailable");
    },
    tone: "negative",
  },
};

export function BackupState({ state }: { state: string }) {
  const { t } = useTranslation();
  const display = states[state] ?? {
    label: t("common.unknown"),
    tone: "neutral",
  };
  return <StatusPill tone={display.tone}>{display.label}</StatusPill>;
}
