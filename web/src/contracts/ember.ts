import type {
  ConfigParser,
  StartupShell,
  NetworkProtocol,
  VariableType,
  VariableFormat,
} from "./domains";
import type { LogAlert, LogMatch } from "./logAlerts";

export const CURRENT_EMBER_SCHEMA = 2;

// Normalize older inputs without mutating them or changing current no-primary choices.
export function normalizeEmber(value: Ember): Ember {
  if (value.schema >= CURRENT_EMBER_SCHEMA) return value;
  const ports = value.required_ports;
  return {
    ...value,
    schema: CURRENT_EMBER_SCHEMA,
    ...(value.schema === 1 &&
    ports?.length &&
    !ports.some((port) => port.primary)
      ? {
          required_ports: ports.map((port, index) => ({
            ...port,
            primary: index === 0,
          })),
        }
      : {}),
  };
}

export interface EmberVariableValidation {
  type: VariableType;
  format?: VariableFormat;
  nullable?: boolean;
  min?: number;
  max?: number;
  min_length?: number;
  max_length?: number;
  choices?: string[];
}

export interface EmberConfigFile {
  path: string;
  parser: ConfigParser;
  values: Record<string, string> | null;
  create?: boolean;
}

export interface EmberVariable {
  name: string;
  default: string;
  required: boolean;
  editable: boolean;
  hidden?: boolean;
  pattern?: string;
  validation?: EmberVariableValidation;
}

export interface Ember {
  savedata_paths?: { path: string; required: boolean }[];
  revision?: string;
  id: string;
  schema: number;
  name: string;
  description: string;
  artUrl?: string;
  updateUrl?: string;
  images: Record<string, string>;
  startup: string;
  startup_commands?: Record<string, string>;
  startup_shell?: StartupShell;
  work_dir: string;
  user: string;
  installer?: { image: string; entrypoint: string; script: string };
  console_commands?: boolean;
  stop?: string;
  ready?: string;
  readiness?: {
    any: LogMatch[];
  };
  configs?: EmberConfigFile[];
  log_alerts?: LogAlert[];
  variables: EmberVariable[] | null;
  required_ports?:
    | {
        container: number;
        protocol: NetworkProtocol;
        description?: string;
        primary?: boolean;
      }[]
    | null;
  origin?: {
    format: string;
    author: string;
    update_url: string;
    warnings: string[] | null;
  };
}
