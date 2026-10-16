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
