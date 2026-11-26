import { useTranslation } from "react-i18next";
import { CrashReason } from "../../contracts/domains";
import type { CrashRecord } from "../../api/crashes";
import { DetailsList, DetailItem } from "../layout/DetailsList";

export function CrashDetails({
  record,
  canReadConsole,
}: {
  record: CrashRecord;
  canReadConsole: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <DetailsList>
        <DetailItem label={t("crashes.exitedAt")}>
          {new Date(record.exited_at * 1000).toLocaleString()}
        </DetailItem>
        <DetailItem label={t("crashes.exitCode")}>
          {record.exit_code}
        </DetailItem>
        <DetailItem label={t("crashes.reason")}>
          {t(
            record.reason === CrashReason.OutOfMemory
              ? "crashes.outOfMemory"
              : "common.unknown",
          )}
        </DetailItem>
      </DetailsList>
      <p className="field-help">{t("crashes.retention")}</p>
      {canReadConsole && (
        <>
          <h3>{t("crashes.consoleTail")}</h3>
          {record.console_unavailable ? (
            <p>{t("crashes.consoleUnavailable")}</p>
          ) : record.console ? (
            <pre className="activity-evidence">{record.console}</pre>
          ) : (
            <p>{t("crashes.consoleEmpty")}</p>
          )}
          {record.console_truncated && (
            <p className="field-help">{t("crashes.consoleLimited")}</p>
          )}
        </>
      )}
    </>
  );
}
