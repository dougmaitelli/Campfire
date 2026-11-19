export function hasExternalFiles(transfer: DataTransfer | null): boolean {
  return transfer?.types.includes("Files") === true;
}
