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
    startup_shell: "bash",
    work_dir: "/home/container",
    user: "1000:1000",
    installer: {
      image: "alpine:3.22",
      entrypoint: "sh",
      script: "echo install",
    },
    stop: "^SIGTERM",
    readiness: {
      strip_ansi: true,
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
          type: "integer",
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
          type: "string",
          format: "url",
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
        parser: "json",
        values: {
          "server.port": "{{SERVER_PORT}}",
          "server.name": "{{LABEL}}",
        },
        create: true,
      },
    ],
    required_ports: [
      { container: 25565, protocol: "tcp", description: "Game" },
      { container: 25566, protocol: "udp", description: "Query" },
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
