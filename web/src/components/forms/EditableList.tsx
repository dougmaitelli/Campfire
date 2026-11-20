import type { Key, ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { ArrowUp, ArrowDown, Plus } from "lucide-react";
import { EditableListRow } from "./EditableListRow";
import { Button } from "./Button";

type Reordering<T> =
  | {
      reorderable: true;
      onMove: (from: number, to: number) => void;
      itemKey: (item: T) => Key;
    }
  | { reorderable?: false; onMove?: never; itemKey?: (item: T) => Key };

export function EditableList<T>({
  items,
  rowLabel,
  removeLabel,
  addLabel,
  onRemove,
  onAdd,
  maxItems = 128,
  addDisabled = false,
  children,
  renderBody,
  className,
  reorderable = false,
  onMove,
  itemKey,
}: {
  items: readonly T[];
  rowLabel: (index: number) => string;
  removeLabel: (index: number) => string;
  addLabel: string;
  onRemove: (index: number) => void;
  onAdd: () => void;
  maxItems?: number;
  addDisabled?: boolean;
  children: (item: T, index: number) => ReactNode;
  renderBody?: (item: T, index: number) => ReactNode;
  className?: string;
} & Reordering<T>) {
  const { t } = useTranslation();
  return (
    <div className={["editable-list", className].filter(Boolean).join(" ")}>
      {items.map((item, index) => (
        <EditableListRow
          key={itemKey ? itemKey(item) : index}
          label={rowLabel(index)}
          removeLabel={removeLabel(index)}
          onRemove={() => onRemove(index)}
          body={renderBody?.(item, index)}
          reorderControls={
            reorderable && (
              <>
                <Button
                  type="button"
                  iconOnly
                  disabled={index === 0}
                  aria-label={t("editableList.moveUp", {
                    name: rowLabel(index),
                  })}
                  title={t("editableList.moveUp", { name: rowLabel(index) })}
                  onClick={() => onMove?.(index, index - 1)}
                >
                  <ArrowUp size={18} aria-hidden="true" />
                </Button>
                <Button
                  type="button"
                  iconOnly
                  disabled={index === items.length - 1}
                  aria-label={t("editableList.moveDown", {
                    name: rowLabel(index),
                  })}
                  title={t("editableList.moveDown", { name: rowLabel(index) })}
                  onClick={() => onMove?.(index, index + 1)}
                >
                  <ArrowDown size={18} aria-hidden="true" />
                </Button>
              </>
            )
          }
        >
          {children(item, index)}
        </EditableListRow>
      ))}
      <Button
        type="button"
        disabled={addDisabled || items.length >= maxItems}
        onClick={onAdd}
      >
        <Plus size={16} aria-hidden="true" />
        {addLabel}
      </Button>
    </div>
  );
}
