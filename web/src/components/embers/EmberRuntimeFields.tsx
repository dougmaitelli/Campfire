import { useTranslation } from "react-i18next";
import { FormField } from "../forms/FormField";
import { Select } from "../forms/Select";
import { AllocationList } from "../forms/AllocationList";
import { EditorCheck, EditorGroup, EditorText } from "./EmberEditorFields";
import type { EmberDraft, NamedValue } from "./emberDraft";
import { EmberLogAlertFields } from "./EmberLogAlertFields";

export function EmberRuntimeFields({
  draft,
  onChange,
}: {
  draft: EmberDraft;
  onChange: (value: EmberDraft) => void;
}) {
  const { t } = useTranslation();
  const value = draft.definition;
  const readiness = draft.readiness;
  function changeReadiness(next: typeof readiness) {
    onChange({ ...draft, readiness: next });
  }
  function patch(next: Partial<typeof value>) {
    onChange({ ...draft, definition: { ...value, ...next } });
  }
  function changeCommand(index: number, fields: Partial<NamedValue>) {
    onChange({
      ...draft,
      commands: draft.commands.map((command, i) =>
        i === index ? { ...command, ...fields } : command,
      ),
    });
  }
  function commandField(command: NamedValue, index: number) {
    return (
      <EditorText
        label={t("emberEditor.startup")}
        value={command.value}
        required
        multiline
        onChange={(value) => changeCommand(index, { value })}
      />
    );
  }
  return (
    <>
      <div className="ember-editor-grid">
        <FormField label={t("emberEditor.shell")}>
          <Select
            value={value.startup_shell ?? "sh"}
            onValueChange={(startup_shell) => patch({ startup_shell })}
          >
            <option value="sh">sh</option>
            <option value="bash">bash</option>
            <option value="ash">ash</option>
          </Select>
        </FormField>
        <EditorText
          label={t("emberEditor.user")}
          value={value.user}
          required
          pattern="[1-9][0-9]*(?::[0-9]+)?"
          placeholder={t("emberEditor.userPlaceholder")}
          onChange={(user) => patch({ user })}
        />
      </div>
      <EditorText
        label={t("emberEditor.workDirectory")}
        value={value.work_dir}
        required
        onChange={(work_dir) => patch({ work_dir })}
      />
      <EditorGroup
        title={t("emberEditor.images")}
        description={t("emberEditor.imagesHelp")}
      >
        <AllocationList
          items={draft.images}
          maxItems={32}
          rowLabel={(index) =>
            t("emberEditor.item", {
              name: t("emberEditor.image"),
              number: index + 1,
            })
          }
          removeLabel={(index) =>
            t("emberEditor.removeItem", {
              name: t("emberEditor.image"),
              number: index + 1,
            })
          }
          addLabel={t("emberEditor.add", { name: t("emberEditor.image") })}
          onRemove={(index) =>
            onChange({
              ...draft,
              images: draft.images.filter((_, i) => i !== index),
            })
          }
          onAdd={() =>
            onChange({
              ...draft,
              images: [...draft.images, { name: "", value: "" }],
            })
          }
        >
          {(image, index) => (
            <>
              <EditorText
                label={t("emberEditor.imageLabel")}
                value={image.name}
                required
                onChange={(name) =>
                  onChange({
                    ...draft,
                    images: draft.images.map((item, i) =>
                      i === index ? { ...item, name } : item,
                    ),
                  })
                }
              />
              <EditorText
                label={t("emberEditor.imageAddress")}
                value={image.value}
                required
                onChange={(value) =>
                  onChange({
                    ...draft,
                    images: draft.images.map((item, i) =>
                      i === index ? { ...item, value } : item,
                    ),
                  })
                }
              />
            </>
          )}
        </AllocationList>
      </EditorGroup>
      <EditorGroup
        title={t("emberEditor.startupCommands")}
        description={t("emberEditor.startupHelp")}
      >
        <AllocationList
          items={draft.commands}
          maxItems={32}
          rowLabel={(index) =>
            t("emberEditor.item", {
              name: t("emberEditor.startupCommand"),
              number: index + 1,
            })
          }
          removeLabel={(index) =>
            t("emberEditor.removeItem", {
              name: t("emberEditor.startupCommand"),
              number: index + 1,
            })
          }
          addLabel={t("emberEditor.add", {
            name: t("emberEditor.startupCommand"),
          })}
          onRemove={(index) =>
            onChange({
              ...draft,
              commands: draft.commands.filter((_, i) => i !== index),
            })
          }
          onAdd={() =>
            onChange({
              ...draft,
              commands: [
                ...draft.commands,
                { name: `Command ${draft.commands.length + 1}`, value: "" },
              ],
            })
          }
          renderBody={(command, index) =>
            draft.commands.length > 1 ? commandField(command, index) : null
          }
        >
          {(command, index) =>
            draft.commands.length > 1 ? (
              <EditorText
                label={t("emberEditor.commandName")}
                value={command.name}
                required
                onChange={(name) => changeCommand(index, { name })}
              />
            ) : (
              commandField(command, index)
            )
          }
        </AllocationList>
      </EditorGroup>
      <EditorText
        label={t("emberEditor.stop")}
        value={value.stop ?? ""}
        description={t("emberEditor.stopHelp")}
        onChange={(stop) => patch({ stop })}
      />
      <EditorGroup title={t("emberEditor.readiness")}>
        <FormField label={t("emberEditor.readinessMode")}>
          <Select
            value={draft.waitForReadyMessage ? "messages" : "running"}
            onValueChange={(mode) =>
              onChange({
                ...draft,
                waitForReadyMessage: mode === "messages",
                readiness:
                  mode === "messages" && !readiness.any.length
                    ? { ...readiness, any: [{ value: "", regex: false }] }
                    : readiness,
              })
            }
          >
            <option value="running">{t("emberEditor.whenRunning")}</option>
            <option value="messages">{t("emberEditor.whenMessage")}</option>
          </Select>
        </FormField>
        {draft.waitForReadyMessage && (
          <>
            <p className="field-help">{t("emberEditor.readyRulesHelp")}</p>
            <AllocationList
              items={readiness.any}
              maxItems={16}
              rowLabel={(index) =>
                t("emberEditor.item", {
                  name: t("emberEditor.readyRule"),
                  number: index + 1,
                })
              }
              removeLabel={(index) =>
                t("emberEditor.removeItem", {
                  name: t("emberEditor.readyRule"),
                  number: index + 1,
                })
              }
              addLabel={t("emberEditor.add", {
                name: t("emberEditor.readyRule"),
              })}
              onRemove={(index) =>
                changeReadiness({
                  ...readiness,
                  any: readiness.any.filter((_, i) => i !== index),
                })
              }
              onAdd={() =>
                changeReadiness({
                  ...readiness,
                  any: [...readiness.any, { value: "", regex: false }],
                })
              }
              renderBody={(match, index) => (
                <div className="ember-editor-checks">
                  <EditorCheck
                    label={t("emberEditor.regularExpression")}
                    value={match.regex}
                    onChange={(regex) =>
                      changeReadiness({
                        ...readiness,
                        any: readiness.any.map((item, i) =>
                          i === index ? { ...item, regex } : item,
                        ),
                      })
                    }
                  />
                  <EditorCheck
                    label={t("logAlerts.ignoreCase")}
                    value={match.ignore_case}
                    onChange={(ignore_case) =>
                      changeReadiness({
                        ...readiness,
                        any: readiness.any.map((item, i) =>
                          i === index ? { ...item, ignore_case } : item,
                        ),
                      })
                    }
                  />
                </div>
              )}
            >
              {(match, index) => (
                <EditorText
                  label={t("emberEditor.matchValue")}
                  value={match.value}
                  required
                  onChange={(value) =>
                    changeReadiness({
                      ...readiness,
                      any: readiness.any.map((item, i) =>
                        i === index ? { ...item, value } : item,
                      ),
                    })
                  }
                />
              )}
            </AllocationList>
          </>
        )}
      </EditorGroup>
      <EmberLogAlertFields draft={draft} onChange={onChange} />
    </>
  );
}
