import type {
  ConfigParser,
  NetworkProtocol,
  VariableType,
  VariableFormat,
} from "./domains";

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
  startup_shell?: "sh" | "bash" | "ash";
  work_dir: string;
  user: string;
  installer?: { image: string; entrypoint: string; script: string };
  stop?: string;
  ready?: string;
  readiness?: {
    any: { value: string; regex?: boolean }[];
    strip_ansi?: boolean;
  };
  configs?: EmberConfigFile[];
  variables: EmberVariable[] | null;
  required_ports?:
    | {
        container: number;
        protocol: NetworkProtocol;
        description?: string;
      }[]
    | null;
  origin?: {
    format: string;
    author: string;
    update_url: string;
    warnings: string[] | null;
  };
}
