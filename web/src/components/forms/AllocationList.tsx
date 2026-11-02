import type { ReactNode } from "react";
import { Plus } from "lucide-react";
import { AllocationRow } from "./AllocationRow";
import { Button } from "./Button";

export function AllocationList<T>({
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
}) {
  return (
    <div className={["allocation-list", className].filter(Boolean).join(" ")}>
      {items.map((item, index) => (
        <AllocationRow
          key={index}
          label={rowLabel(index)}
          removeLabel={removeLabel(index)}
          onRemove={() => onRemove(index)}
          body={renderBody?.(item, index)}
        >
          {children(item, index)}
        </AllocationRow>
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
