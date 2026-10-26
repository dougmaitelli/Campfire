import {
  Archive,
  ArchiveRestore,
  ArrowRight,
  Bell,
  CalendarClock,
  Check,
  ChevronUp,
  CircleCheck,
  CircleX,
  ClipboardList,
  Cpu,
  Download,
  File,
  FilePlus,
  Flame,
  Folder,
  FolderInput,
  FolderPlus,
  HardDrive,
  KeyRound,
  LayoutDashboard,
  Link,
  LoaderCircle,
  LogOut,
  MemoryStick,
  Play,
  RotateCcw,
  Search,
  Send,
  Server,
  ShieldCheck,
  Square,
  SquarePen,
  SquareX,
  Table,
  Terminal,
  Trash2,
  TriangleAlert,
  Upload,
  Users,
  X,
  Zap,
  type LucideIcon,
} from "lucide-react";
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
  | "permissions"
  | "delete"
  | "warning"
  | "table"
  | "arrow-right"
  | "terminal"
  | "play"
  | "stop"
  | "restart"
  | "kill"
  | "folder"
  | "upload"
  | "file-plus"
  | "file"
  | "folder-plus"
  | "move"
  | "send"
  | "cpu"
  | "memory"
  | "disk"
  | "search"
  | "check-circle"
  | "x-circle";

const icons: Record<NavigationIconName, LucideIcon> = {
  move: FolderInput,
  upload: Upload,
  "file-plus": FilePlus,
  "folder-plus": FolderPlus,
  folder: Folder,
  file: File,
  restart: RotateCcw,
  kill: SquareX,
  send: Send,
  check: Check,
  close: X,
  "check-circle": CircleCheck,
  "x-circle": CircleX,
  search: Search,
  cpu: Cpu,
  memory: MemoryStick,
  disk: HardDrive,
  play: Play,
  stop: Square,
  "arrow-right": ArrowRight,
  terminal: Terminal,
  backups: Archive,
  download: Download,
  compress: Archive,
  extract: ArchiveRestore,
  rename: SquarePen,
  permissions: ShieldCheck,
  delete: Trash2,
  loading: LoaderCircle,
  warning: TriangleAlert,
  table: Table,
  logout: LogOut,
  chevron: ChevronUp,
  key: KeyRound,
  link: Link,
  notifications: Bell,
  audit: ClipboardList,
  users: Users,
  roles: ShieldCheck,
  dashboard: LayoutDashboard,
  sparks: Zap,
  embers: Flame,
  servers: Server,
  schedules: CalendarClock,
};

export function NavigationIcon({
  name,
  filled = false,
}: {
  name: NavigationIconName;
  filled?: boolean;
}) {
  const Icon = icons[name];
  return <Icon fill={filled ? "currentColor" : "none"} aria-hidden="true" />;
}
