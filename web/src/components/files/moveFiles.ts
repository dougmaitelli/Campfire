import i18n from "../../i18n";
import { manageFiles } from "../../api/files";
import { FileOperation } from "../../contracts/domains";
import { publicationUnconfirmed } from "./publication";

export function canMoveItems(
  directory: string,
  names: readonly string[],
  destination: string,
): boolean {
  return (
    names.length > 0 &&
    destination !== directory &&
    names.every((name) => !destination.startsWith(`${directory}${name}/`))
  );
}

/** Explicit Move and drop-to-move share preflight, sequential execution, and failure reporting. */
export async function moveFiles({
  serverID,
  directory,
  names,
  destination,
  onMoved,
}: {
  serverID: string;
  directory: string;
  names: readonly string[];
  destination: string;
  onMoved: (name: string) => void;
}): Promise<void> {
  const items = [...new Set(names)];
  if (!items.length) return;
  const target = await manageFiles(serverID, {
    operation: FileOperation.List,
    path: destination,
  });
  if (target.path === directory)
    throw new Error(i18n.t("serverFileBrowser.chooseDifferentFolder"));
  if (!canMoveItems(directory, items, target.path))
    throw new Error(i18n.t("serverFileBrowser.invalidMoveDestination"));
  if (target.entries?.some((entry) => items.includes(entry.name)))
    throw new Error(i18n.t("serverFileBrowser.destinationConflict"));
  let confirmed = 0;
  for (const name of items) {
    try {
      await manageFiles(serverID, {
        operation: FileOperation.Rename,
        path: `${directory}${name}`,
        destination: `${target.path}${name}`,
      });
    } catch (cause) {
      throw new Error(
        i18n.t(
          publicationUnconfirmed(cause)
            ? "serverFileBrowser.moveUnconfirmed"
            : "serverFileBrowser.moveStopped",
          {
            confirmed,
            total: items.length,
            name,
            message:
              cause instanceof Error
                ? cause.message
                : i18n.t("serverFileBrowser.fileOperationFailed"),
          },
        ),
        { cause },
      );
    }
    confirmed += 1;
    onMoved(name);
  }
}
