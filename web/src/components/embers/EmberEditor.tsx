import { useTranslation } from "react-i18next";
import type { ReactNode } from "react";
import { Wizard } from "../wizard/Wizard";
import { EmberBasicsFields } from "./EmberBasicsFields";
import { EmberRuntimeFields } from "./EmberRuntimeFields";
import { EmberInstallationFields } from "./EmberInstallationFields";
import { EmberVariableFields } from "./EmberVariableFields";
import { EmberFileFields } from "./EmberFileFields";
import { EmberPortFields } from "./EmberPortFields";
import { EmberStorageFields } from "./EmberStorageFields";
import { emberDraftIssue, type EmberDraft } from "./emberDraft";
import "./ember-editor.css";

export function EmberEditor({
  draft,
  onChange,
  busy,
  error,
  ready,
  editing,
  finishLabel,
  onFinish,
  review,
  children,
}: {
  draft: EmberDraft;
  onChange: (value: EmberDraft) => void;
  busy: boolean;
  error: string | null;
  ready: boolean;
  editing: boolean;
  finishLabel: string;
  onFinish: () => Promise<void>;
  review: ReactNode;
  children: (content: ReactNode, actions: ReactNode) => ReactNode;
}) {
  const { t } = useTranslation();
  const fields = { draft, onChange };
  return (
    <Wizard
      allowAllSteps={editing}
      busy={busy}
      error={error}
      finishLabel={finishLabel}
      onFinish={onFinish}
      steps={[
        {
          id: "basics",
          title: t("emberEditor.basics"),
          content: <EmberBasicsFields {...fields} />,
        },
        {
          id: "installation",
          title: t("emberEditor.installation"),
          content: <EmberInstallationFields {...fields} />,
        },
        {
          id: "runtime",
          title: t("emberEditor.runtime"),
          content: <EmberRuntimeFields {...fields} />,
          validate: () =>
            !draft.images.length
              ? t("emberEditor.needImage")
              : !draft.commands.length
                ? t("emberEditor.needCommand")
                : emberDraftIssue(draft, "runtime"),
        },
        {
          id: "variables",
          title: t("emberEditor.variables"),
          content: <EmberVariableFields {...fields} />,
        },
        {
          id: "ports",
          title: t("emberEditor.portsStep"),
          content: <EmberPortFields {...fields} />,
        },
        {
          id: "files",
          title: t("emberEditor.files"),
          content: <EmberFileFields {...fields} />,
          validate: () => emberDraftIssue(draft, "files"),
        },
        {
          id: "storage",
          title: t("emberEditor.storage"),
          content: <EmberStorageFields {...fields} />,
        },
        {
          id: "review",
          title: t("emberEditor.review"),
          content: (
            <>
              <dl className="ember-editor-summary">
                <div>
                  <dt>{t("common.name")}</dt>
                  <dd>{draft.definition.name}</dd>
                </div>
                <div>
                  <dt>{t("emberEditor.images")}</dt>
                  <dd>{draft.images.length}</dd>
                </div>
                <div>
                  <dt>{t("emberEditor.variables")}</dt>
                  <dd>{draft.definition.variables?.length ?? 0}</dd>
                </div>
                <div>
                  <dt>{t("emberEditor.files")}</dt>
                  <dd>{draft.files.length}</dd>
                </div>
                <div>
                  <dt>{t("emberEditor.ports")}</dt>
                  <dd>{draft.definition.required_ports?.length ?? 0}</dd>
                </div>
                <div>
                  <dt>{t("emberEditor.savedata")}</dt>
                  <dd>{draft.definition.savedata_paths?.length ?? 0}</dd>
                </div>
              </dl>
              {review}
              {draft.definition.origin && (
                <details className="ember-editor-group">
                  <summary>{t("emberEditor.origin")}</summary>
                  <dl className="ember-editor-summary">
                    <div>
                      <dt>{t("emberEditor.format")}</dt>
                      <dd>{draft.definition.origin.format}</dd>
                    </div>
                    <div>
                      <dt>{t("emberEditor.author")}</dt>
                      <dd>{draft.definition.origin.author}</dd>
                    </div>
                    <div>
                      <dt>{t("emberEditor.declaredURL")}</dt>
                      <dd>{draft.definition.origin.update_url || "—"}</dd>
                    </div>
                  </dl>
                  <ul>
                    {(draft.definition.origin.warnings ?? []).map(
                      (warning, index) => (
                        <li key={index}>{warning}</li>
                      ),
                    )}
                  </ul>
                </details>
              )}
            </>
          ),
          validate: () =>
            emberDraftIssue(draft) ??
            (ready ? null : t("emberEditor.reviewNeeded")),
        },
      ]}
    >
      {(content, actions) =>
        children(<div className="ember-editor">{content}</div>, actions)
      }
    </Wizard>
  );
}
