import type { NavigationIcon as IconName } from "../../routing/navigation";

type NavigationIconName =
  | IconName
  | "campfire"
  | "logout"
  | "chevron"
  | "key"
  | "link"
  | "loading"
  | "download"
  | "compress"
  | "extract"
  | "rename"
  | "delete"
  | "warning"
  | "table"
  | "arrow-right";

const paths: Record<NavigationIconName, string> = {
  "arrow-right": "M4 12h16 m-6-6 6 6-6 6",
  backups: "M3 3h18v5H3z M5 8v13h14V8 M9 12h6",
  download: "M12 3v12 m-5-5 5 5 5-5 M4 16v5h16v-5",
  compress: "M3 4h18v5H3z M5 9v12h14V9 M9 13h6 M12 13v5 m-2-2 2 2 2-2",
  extract: "M3 4h18v5H3z M5 9v12h14V9 M12 18v-6 m-3 3 3-3 3 3",
  rename: "m16 3 5 5-12 12-6 1 1-6L16 3Z M13 6l5 5",
  delete: "M3 6h18 M9 6V3h6v3 M5 6l1 15h12l1-15 M10 10v7 M14 10v7",
  loading: "M20 12a8 8 0 1 1-8-8",
  warning: "M12 3 2 21h20L12 3Z M12 9v5 M12 17h.01",
  table: "M3 5h18v14H3z M3 10h18 M9 10v9",
  logout: "M10 17l5-5-5-5 M15 12H3 M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4",
  chevron: "m9 14 3-3 3 3",
  key: "M10 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0Z M10 12h11 M17 12v4 M21 12v4",
  link: "M10 13a5 5 0 0 0 7 0l3-3a5 5 0 0 0-7-7l-2 2 M14 11a5 5 0 0 0-7 0l-3 3a5 5 0 0 0 7 7l2-2",
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

export function NavigationIcon({ name }: { name: NavigationIconName }) {
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
