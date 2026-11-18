import { useTranslation } from "react-i18next";
import type { PublicationStatus } from "../../api/fileTypes";
import { summarizeFileOutcomes, type FileOutcome } from "./fileOutcomes";

export function FileResults({
  outcomes,
  operation = "upload",
}: {
  outcomes: FileOutcome[];
  operation?: "upload" | "extract";
}) {
  const { t } = useTranslation();
  function outcomeLabel(status: PublicationStatus) {
    switch (status) {
      case "extracted":
      case "merged":
        return t(`fileExtraction.outcome.${status}`);
      case "renamed":
      case "cancelled":
        return t(
          `${operation === "extract" ? "fileExtraction" : "fileUploads"}.outcome.${status}`,
        );
      default:
        return t(`fileUploads.outcome.${status}`);
    }
  }
  const summary = summarizeFileOutcomes(outcomes);
  const namespace = operation === "extract" ? "fileExtraction" : "fileUploads";
  return (
    <details>
      <summary>
        {t(
          summary.unconfirmed > 0
            ? `${namespace}.resultsUnconfirmed`
            : `${namespace}.results`,
          summary,
        )}
      </summary>
      <ul>
        {outcomes.map((item, index) => (
          <li key={index}>
            <code>{item.name}</code>
            {item.target !== item.name && (
              <>
                {" "}
                → <code>{item.target}</code>
              </>
            )}
            {": "}
            {outcomeLabel(item.status)}
            {item.message && ` — ${item.message}`}
          </li>
        ))}
      </ul>
    </details>
  );
}
