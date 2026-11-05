import type { Backup } from "../api/backups";

export function backupDisplayName(backup: Backup): string {
  return backup.display_name || backup.name;
}
