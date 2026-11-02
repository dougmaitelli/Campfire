import { useTranslation } from "react-i18next";
import type { EmberPreview } from "../../api/embers";
import { CheckboxField } from "../forms/CheckboxField";
import { InlineMessage } from "../status/InlineMessage";

export function EmberCompatibilityPreview({
  preview,
  checking,
  warnings,
  reviewed,
  onReviewed,
  showDefinition,
}: {
  preview: EmberPreview | null;
  checking: boolean;
  warnings: EmberPreview["warnings"];
  reviewed: boolean;
  onReviewed: (value: boolean) => void;
  showDefinition: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      {checking && (
        <p role="status">{t("importEmberDrawer.checkingCompatibility")}</p>
      )}
      {preview && (
        <section aria-label={t("importEmberDrawer.compatibilityPreview")}>
          <h3>
            {preview.errors.length > 0
              ? t("importEmberDrawer.definitionNeedsChanges")
              : t("importEmberDrawer.definitionIsValid")}
          </h3>
          {preview.errors.length > 0 && (
            <>
              <InlineMessage role="alert">
                {t("importEmberDrawer.resolveErrorsBeforeContinuing")}
              </InlineMessage>
              <ul>
                {preview.errors.map((issue, index) => (
                  <li key={`${issue.field}-${index}`}>
                    <strong>{issue.field}:</strong> {issue.message}
                  </li>
                ))}
              </ul>
            </>
          )}
          {warnings.length > 0 && (
            <>
              <p>{t("importEmberDrawer.beforeRunningAServerWithThisEmber")}</p>
              <ul>
                {warnings.map((issue, index) => (
                  <li key={`${issue.field}-${index}`}>{issue.message}</li>
                ))}
              </ul>
              <CheckboxField
                checked={reviewed}
                onChange={(event) => onReviewed(event.target.checked)}
              >
                {t("importEmberDrawer.iUnderstandTheseSetupRequirements")}
              </CheckboxField>
            </>
          )}
          {showDefinition && preview.format !== "ember" && preview.ember && (
            <details>
              <summary>{t("importEmberDrawer.convertedEmber")}</summary>
              <pre className="ember-preview-definition">
                {JSON.stringify(preview.ember, null, 2)}
              </pre>
            </details>
          )}
        </section>
      )}
    </>
  );
}
