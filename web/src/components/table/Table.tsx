import type { ComponentProps } from "react";
import "./table.css";

export function Table({
  className,
  children,
  ...props
}: ComponentProps<"table">) {
  return (
    <div className="table-container">
      <table
        {...props}
        className={className ? `data-table ${className}` : "data-table"}
      >
        {children}
      </table>
    </div>
  );
}

export function TableRow({
  onOpen,
  className,
  children,
  ...props
}: ComponentProps<"tr"> & { onOpen: () => void }) {
  return (
    <tr
      {...props}
      className={`table-row-interactive${className ? ` ${className}` : ""}`}
      onClick={(event) => {
        if (
          event.target instanceof Element &&
          event.target.closest(
            "a, button, input, select, textarea, summary, details, [role=button]",
          )
        )
          return;
        if (window.getSelection()?.toString()) return;
        onOpen();
      }}
    >
      {children}
    </tr>
  );
}
