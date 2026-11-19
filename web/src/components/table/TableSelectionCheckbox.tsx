import { useTranslation } from "react-i18next";
import { CheckboxField } from "../forms/CheckboxField";
import type { TableSelection } from "./useTableSelection";

/** Omit id for the header's select-all checkbox. */
export function TableSelectionCheckbox({
  selection,
  id,
  label,
}: {
  selection: TableSelection;
  id?: string;
  label?: string;
}) {
  const { t } = useTranslation();
  const all = id === undefined;
  const checked = all
    ? selection.ids.length > 0 &&
      selection.selectedIDs.length === selection.ids.length
    : selection.isSelected(id);
  return (
    <CheckboxField
      ref={(input) => {
        if (input)
          input.indeterminate =
            all && !checked && selection.selectedIDs.length > 0;
      }}
      checked={checked}
      disabled={
        selection.disabled ||
        (all ? !selection.ids.length : !selection.isSelectable(id))
      }
      onChange={() => {
        /* Click carries modifiers for mouse and keyboard activation. */
      }}
      onClick={(event) => {
        event.stopPropagation();
        if (all) {
          if (event.currentTarget.checked)
            selection.setSelection(selection.ids);
          else selection.clear();
        } else
          selection.select(id, {
            additive: true,
            shift: event.shiftKey,
            checked: event.currentTarget.checked,
          });
      }}
    >
      <span className="visually-hidden">
        {label ??
          t(all ? "table.selectAll" : "table.selectEntry", { name: id })}
      </span>
    </CheckboxField>
  );
}
