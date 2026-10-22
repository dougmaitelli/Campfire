import type { NavigationIcon as IconName } from "../../routing/navigation";

type NavigationIconName =
  | IconName
  | "logout"
  | "chevron"
  | "close"
  | "check"
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
  | "arrow-right"
  | "play"
  | "stop"
  | "restart"
  | "kill"
  | "folder"
  | "upload"
  | "file-plus"
  | "folder-plus"
  | "move"
  | "send"
  | "cpu"
  | "memory"
  | "disk"
  | "search"
  | "check-circle"
  | "x-circle";

const paths: Record<NavigationIconName, string> = {
  move: "M3 7V4h6l2 3h10v13H3V7Z M8 14h8 m-3-3 3 3-3 3",
  upload: "M12 16V3 m-5 5 5-5 5 5 M4 16v5h16v-5",
  "file-plus": "M14 2H4v20h16V8l-6-6Z M14 2v6h6 M8 15h8 M12 11v8",
  "folder-plus": "M3 7V4h6l2 3h10v13H3V7Z M8 14h8 M12 10v8",
  folder: "M3 7V4h6l2 3h10v13H3V7Z",
  restart: "M21 12a9 9 0 1 1-2.64-6.36L21 8 M21 3v5h-5",
  kill: "M3 3h18v18H3V3Z M8 8l8 8 M16 8l-8 8",
  send: "m3 3 19 9-19 9 4-9-4-9Z",
  check: "m5 12 4 4L19 6",
  close: "M6 6l12 12M18 6 6 18",
  "check-circle": "M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z m-13 0 3 3 5-6",
  "x-circle": "M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z M9 9l6 6 M15 9l-6 6",
  search: "M16 10a6 6 0 1 1-12 0 6 6 0 0 1 12 0Z m-2 4 6 6",
  cpu: "M6 6h12v12H6z M9 9h6v6H9z M9 2v4 M15 2v4 M9 18v4 M15 18v4 M2 9h4 M2 15h4 M18 9h4 M18 15h4",
  memory:
    "M3 6h18v12H3z M7 9v5 M12 9v5 M17 9v5 M6 18v3 M10 18v3 M14 18v3 M18 18v3",
  disk: "M6 3h12l3 12v6H3v-6L6 3Z M3 15h18 M16 18h2",
  play: "m8 5 11 7-11 7V5Z",
  stop: "M6 6h12v12H6z",
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
  chevron: "m7.5 14.25 4.5-4.5 4.5 4.5",
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
};

export function NavigationIcon({
  name,
  filled = false,
}: {
  name: NavigationIconName;
  filled?: boolean;
}) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill={filled ? "currentColor" : "none"}
      stroke={filled ? "none" : "currentColor"}
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={paths[name]} />
    </svg>
  );
}
