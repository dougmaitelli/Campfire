import i18n from "../i18n";

export function formatUserLabel(user: {
  name: string;
  username: string;
}): string {
  return user.name
    ? i18n.t("users.displayLabel", { name: user.name, username: user.username })
    : user.username;
}
