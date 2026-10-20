import i18n from "../../i18n";
import { useTranslation } from "react-i18next";
import type { ComponentProps, ReactNode } from "react";
import { FormField } from "./FormField";
import { TextInput } from "./TextInput";
import { NavigationIcon } from "../navigation/NavigationIcon";
import "./search-field.css";

function SearchInput(props: Omit<ComponentProps<"input">, "type">) {
  return (
    <div className="search-input">
      <NavigationIcon name="search" />
      <TextInput {...props} type="search" />
    </div>
  );
}

export function SearchField({
  label = i18n.t("common.search"),
  ...props
}: Omit<ComponentProps<"input">, "type" | "children"> & { label?: ReactNode }) {
  useTranslation();
  return (
    <FormField label={label} className="search-field">
      <SearchInput {...props} />
    </FormField>
  );
}
