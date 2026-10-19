export function formatUserLabel(user: {
  name: string;
  username: string;
}): string {
  return user.name ? `${user.name} (${user.username})` : user.username;
}
