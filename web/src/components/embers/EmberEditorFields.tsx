import { useTranslation } from "react-i18next";
import type { ComponentProps, ReactNode } from "react";
import { useId } from "react";
import { FormField } from "../forms/FormField";
import { TextInput } from "../forms/TextInput";
import { Textarea } from "../forms/Textarea";
import { CheckboxField } from "../forms/CheckboxField";
import { AllocationList } from "../forms/AllocationList";
import type { NamedValue } from "./emberDraft";

export function EditorText({
  label,
  value,
  onChange,
  multiline = false,
  description,
  ...props
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  multiline?: boolean;
  description?: string;
} & Pick<
  ComponentProps<"input">,
  "required" | "placeholder" | "maxLength" | "pattern" | "type"
>) {
  return (
    <FormField label={label} {...(description ? { description } : {})}>
      {multiline ? (
        <Textarea
          rows={4}
          className="ember-code"
          value={value}
          required={props.required}
          placeholder={props.placeholder}
          maxLength={props.maxLength}
          spellCheck={false}
          onChange={(event) => onChange(event.target.value)}
        />
      ) : (
        <TextInput
          {...props}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </FormField>
  );
}

export function EditorCheck({
  label,
  value,
  onChange,
}: {
  label: string;
  value: boolean | undefined;
  onChange: (value: boolean) => void;
}) {
  return (
    <CheckboxField
      checked={value ?? false}
      onChange={(event) => onChange(event.target.checked)}
    >
      {label}
    </CheckboxField>
  );
}

export function EditorGroup({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <section className="ember-editor-group" aria-label={title}>
      <h4>{title}</h4>
      {description && <p className="field-help">{description}</p>}
      {children}
    </section>
  );
}

export function ReplacementValues({
  rows,
  onChange,
}: {
  rows: NamedValue[];
  onChange: (rows: NamedValue[]) => void;
}) {
  const { t } = useTranslation();
  const title = t("emberEditor.replacement");
  return (
    <AllocationList
      items={rows}
      rowLabel={(index) =>
        t("emberEditor.item", { name: title, number: index + 1 })
      }
      removeLabel={(index) =>
        t("emberEditor.removeItem", { name: title, number: index + 1 })
      }
      addLabel={t("emberEditor.add", { name: title })}
      onRemove={(index) => onChange(rows.filter((_, i) => i !== index))}
      onAdd={() => onChange([...rows, { name: "", value: "" }])}
    >
      {(row, index) => (
        <>
          <EditorText
            label={t("emberEditor.replacementKey")}
            value={row.name}
            required
            onChange={(name) =>
              onChange(
                rows.map((item, i) => (i === index ? { ...item, name } : item)),
              )
            }
          />
          <EditorText
            label={t("emberEditor.replacementValue")}
            value={row.value}
            onChange={(value) =>
              onChange(
                rows.map((item, i) =>
                  i === index ? { ...item, value } : item,
                ),
              )
            }
          />
        </>
      )}
    </AllocationList>
  );
}

export function AllowedValues({
  values,
  onChange,
}: {
  values: string[];
  onChange: (values: string[]) => void;
}) {
  const { t } = useTranslation();
  const listID = useId();
  const title = t("emberEditor.choice");
  return (
    <div role="group" aria-labelledby={listID}>
      <div className="ember-list-label" id={listID}>
        {t("emberEditor.allowedValues")}
      </div>
      <AllocationList
        className="ember-value-list"
        items={values}
        rowLabel={(index) =>
          t("emberEditor.item", { name: title, number: index + 1 })
        }
        removeLabel={(index) =>
          t("emberEditor.removeItem", { name: title, number: index + 1 })
        }
        addLabel={t("emberEditor.add", { name: title })}
        onRemove={(index) => onChange(values.filter((_, i) => i !== index))}
        onAdd={() => onChange([...values, ""])}
      >
        {(value, index) => (
          <TextInput
            aria-label={t("emberEditor.item", {
              name: title,
              number: index + 1,
            })}
            value={value}
            onChange={(event) =>
              onChange(
                values.map((item, i) =>
                  i === index ? event.target.value : item,
                ),
              )
            }
          />
        )}
      </AllocationList>
    </div>
  );
}
