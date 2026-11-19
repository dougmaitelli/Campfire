import {
  StartupShell,
  VariableType,
  VariableFormat,
  ConfigParser,
  NetworkProtocol,
} from "../../src/contracts/domains";
import type { Ember } from "../../src/contracts/ember";

export function completeEmber(): Ember {
  return {
    id: "editor-fixture",
    revision: "existing-revision",
    schema: 1,
    name: "Authoring fixture",
    description: "Every supported authoring option.",
    artUrl: "https://example.com/art.png",
    updateUrl: "https://example.com/current.yaml",
    images: { Alpine: "alpine:3.22", Debian: "debian:bookworm-slim" },
    startup: "",
    startup_commands: {
      Default: "echo Ready; sleep {{SECONDS}}",
      Debug: "echo Debug; sleep {{SECONDS}}",
    },
    startup_shell: StartupShell.Bash,
    work_dir: "/home/container",
    user: "1000:1000",
    installer: {
      image: "alpine:3.22",
      entrypoint: "sh",
      script: "echo install",
    },
    stop: "^SIGTERM",
    readiness: {
      any: [
        { value: "^Ready$", regex: true },
        { value: "Listening", regex: false },
      ],
    },
    variables: [
      {
        name: "SECONDS",
        default: "30",
        required: true,
        editable: true,
        validation: {
          type: VariableType.Integer,
          min: 1,
          max: 300,
          choices: ["30", "60"],
        },
      },
      {
        name: "URL",
        default: "https://example.com/content",
        required: false,
        editable: false,
        hidden: true,
        pattern: "^https://",
        validation: {
          type: VariableType.String,
          format: VariableFormat.URL,
          nullable: true,
          min_length: 1,
          max_length: 8192,
          choices: ["https://example.com/content", ""],
        },
      },
      { name: "LABEL", default: "Default", required: false, editable: true },
    ],
    configs: [
      {
        path: "settings.json",
        parser: ConfigParser.JSON,
        values: {
          "server.port": "{{SERVER_PORT}}",
          "server.name": "{{LABEL}}",
        },
        create: true,
      },
    ],
    required_ports: [
      { container: 25565, protocol: NetworkProtocol.TCP, description: "Game" },
      { container: 25566, protocol: NetworkProtocol.UDP, description: "Query" },
    ],
    savedata_paths: [
      { path: "world", required: true },
      { path: "players", required: false },
    ],
    origin: {
      format: "PLCN_v3",
      author: "fixtures@example.invalid",
      update_url: "https://example.com/old.json",
      warnings: ["Review the game's setup requirements."],
    },
  };
}
