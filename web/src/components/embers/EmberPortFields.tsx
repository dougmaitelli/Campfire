import { useTranslation } from "react-i18next";
import { NetworkProtocol } from "../../contracts/domains";
import { FormField } from "../forms/FormField";
import { Select } from "../forms/Select";
import { TextInput } from "../forms/TextInput";
import { AllocationList } from "../forms/AllocationList";
import { EditorText } from "./EmberEditorFields";
import type { EmberDraft } from "./emberDraft";

export function EmberPortFields({
  draft,
  onChange,
}: {
  draft: EmberDraft;
  onChange: (value: EmberDraft) => void;
}) {
  const { t } = useTranslation();
  const value = draft.definition;
  const ports = value.required_ports ?? [];
  function patch(next: Partial<typeof value>) {
    onChange({ ...draft, definition: { ...value, ...next } });
  }
  return (
    <>
      <p className="field-help">{t("emberEditor.portsHelp")}</p>
      <AllocationList
        items={ports}
        maxItems={32}
        rowLabel={(index) =>
          t("emberEditor.item", {
            name: t("emberEditor.port"),
            number: index + 1,
          })
        }
        removeLabel={(index) =>
          t("emberEditor.removeItem", {
            name: t("emberEditor.port"),
            number: index + 1,
          })
        }
        addLabel={t("emberEditor.add", { name: t("emberEditor.port") })}
        onRemove={(index) =>
          patch({ required_ports: ports.filter((_, i) => i !== index) })
        }
        onAdd={() =>
          patch({
            required_ports: [
              ...ports,
              { container: 0, protocol: NetworkProtocol.TCP },
            ],
          })
        }
      >
        {(port, index) => (
          <>
            <FormField label={t("emberEditor.containerPort")}>
              <TextInput
                type="number"
                required
                min={1}
                max={65535}
                step={1}
                value={port.container || ""}
                onChange={(event) =>
                  patch({
                    required_ports: ports.map((item, i) =>
                      i === index
                        ? { ...item, container: Number(event.target.value) }
                        : item,
                    ),
                  })
                }
              />
            </FormField>
            <FormField label={t("emberEditor.protocol")}>
              <Select
                value={port.protocol}
                onValueChange={(protocol) =>
                  patch({
                    required_ports: ports.map((item, i) =>
                      i === index ? { ...item, protocol } : item,
                    ),
                  })
                }
              >
                {Object.entries(NetworkProtocol).map(([label, protocol]) => (
                  <option key={protocol} value={protocol}>
                    {label}
                  </option>
                ))}
              </Select>
            </FormField>
            <EditorText
              label={t("common.description")}
              value={port.description ?? ""}
              maxLength={80}
              onChange={(description) =>
                patch({
                  required_ports: ports.map((item, i) =>
                    i === index ? { ...item, description } : item,
                  ),
                })
              }
            />
          </>
        )}
      </AllocationList>
    </>
  );
}
