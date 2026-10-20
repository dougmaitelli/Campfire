import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import type { ComponentProps, ReactNode } from "react";
import { FormField } from "./FormField";
import { TextInput } from "./TextInput";

export function SearchField({
  label = i18n.t("common.search"),
  ...props
}: Omit<ComponentProps<"input">, "type" | "children"> & { label?: ReactNode }) {
  useTranslation();
  return (
    <FormField label={label} className="search-field">
      <TextInput {...props} type="search" />
    </FormField>
  );
}
