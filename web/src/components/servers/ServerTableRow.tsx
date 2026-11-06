import type { ComponentProps, ReactNode } from "react";
import { TableRow } from "../table/Table";
import "./server-table.css";

export function ServerTableRow({
  gameReadings,
  columnCount,
  onOpen,
  ...props
}: ComponentProps<typeof TableRow> & {
  gameReadings?: ReactNode;
  columnCount: number;
}) {
  return (
    <>
      <TableRow {...props} onOpen={onOpen} />
      {gameReadings && (
        <TableRow
          className="server-game-state-row"
          onOpen={onOpen}
          showCaret={false}
        >
          <td colSpan={columnCount}>{gameReadings}</td>
        </TableRow>
      )}
    </>
  );
}
