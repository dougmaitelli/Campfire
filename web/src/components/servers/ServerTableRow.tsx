import type { ComponentProps, ReactNode } from "react";
import { TableRow } from "../table/Table";
import "./server-table.css";

export function ServerTableRow({
  gameReadings,
  columnCount,
  ...props
}: ComponentProps<typeof TableRow> & {
  gameReadings?: ReactNode;
  columnCount: number;
}) {
  return (
    <>
      <TableRow {...props} />
      {gameReadings && (
        <tr className="server-game-state-row">
          <td colSpan={columnCount}>{gameReadings}</td>
        </tr>
      )}
    </>
  );
}
