import type { NavigationIcon as IconName } from "../../routing/navigation";

const paths: Record<IconName | "campfire", string> = {
  notifications: "M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9 M10 21h4",
  audit: "M5 3h14v18H5z M8 7h8 M8 11h8 M8 15h5",
  users:
    "M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2 M16 3a4 4 0 0 1 0 8 M22 21v-2a4 4 0 0 0-3-3.87 M13 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0Z",
  roles: "M12 3 3 7v5c0 5 9 9 9 9s9-4 9-9V7z M8 12l3 3 5-6",
  dashboard: "M3 3h7v7H3z M14 3h7v7h-7z M3 14h7v7H3z M14 14h7v7h-7z",
  sparks: "M13 2 4 14h7l-1 8 10-13h-7z",
  embers:
    "M12 3c1 5 6 6 6 11a6 6 0 0 1-12 0c0-3 2-5 3-7 0 3 2 4 3 4 1-2 1-5 0-8Z",
  servers:
    "M4 3h16v7H4z M4 14h16v7H4z M7 6.5h.01 M7 17.5h.01 M11 6.5h6 M11 17.5h6",
  schedules: "M9 4H5v17h14V4h-4 M9 2h6v4H9z M8 11l2 2 5-5 M8 17h8",
  campfire:
    "M12 2c1 4 6 6 6 10a6 6 0 0 1-12 0c0-2 1-4 3-6 0 3 2 4 3 4 1-2 1-5 0-8Z M4 19l16 3 M20 19 4 22",
};

export function NavigationIcon({ name }: { name: IconName | "campfire" }) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={paths[name]} />
    </svg>
  );
}
