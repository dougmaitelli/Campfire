---
title: Ember schema
description: Native Ember schema 1 fields, examples, defaults, and validation rules.
---

This reference describes native **schema 1** for authoring game definitions. For
creating, importing, and updating definitions in the panel, see [Embers](/Campfire/embers/).

## Native schema

Definitions can be JSON or YAML, up to 1 MiB. JSON property names are case-sensitive:
`artUrl` and `updateUrl` use camel case; other compound names use underscores.

This small definition runs a diagnostic process rather than a real game:

```json
{
  "schema": 1,
  "name": "Example server",
  "description": "A minimal Ember for checking startup and console output.",
  "images": { "Alpine": "alpine:3.22" },
  "startup": "echo Ready; exec sleep 3600",
  "startup_shell": "sh",
  "work_dir": "/data",
  "user": "1000:1000",
  "variables": [],
  "stop": "^SIGTERM",
  "ready": "Ready"
}
```

The examples below are **top-level fragments**, not complete definitions. Merge
relevant fields into one Ember; do not combine mutually exclusive alternatives.

| Property           | Type    | Purpose                                                           |
| ------------------ | ------- | ----------------------------------------------------------------- |
| `schema`           | integer | Required; `1`                                                     |
| `name`             | string  | Required display name                                             |
| `description`      | string  | Description; empty if omitted                                     |
| `artUrl`           | string  | Optional artwork URL or panel asset path                          |
| `updateUrl`        | string  | Optional definition update source                                 |
| `id`, `revision`   | string  | Panel-managed identity and content revision                       |
| `origin`           | object  | Optional import attribution and compatibility notes               |
| `images`           | object  | Required nonempty map of image labels to container references     |
| `startup`          | string  | One startup command; alternative to `startup_commands`            |
| `startup_commands` | object  | Named startup alternatives; cannot accompany a nonempty `startup` |
| `startup_shell`    | string  | `sh` (default), `bash`, or `ash`                                  |
| `work_dir`         | string  | Required absolute container working directory                     |
| `user`             | string  | Required numeric non-root `UID` or `UID:GID`                      |
| `variables`        | array   | Variable definitions; empty if omitted                            |
| `installer`        | object  | Optional installation image, entrypoint, and script               |
| `stop`             | string  | Console command or signal notation; image default if omitted      |
| `ready`            | string  | Simple readiness marker; alternative to `readiness`               |
| `readiness`        | object  | Structured readiness matchers                                     |
| `log_alerts`       | array   | Actionable error rules                                            |
| `configs`          | array   | Configuration-file replacements                                   |
| `required_ports`   | array   | Container port requirements                                       |
| `savedata_paths`   | array   | Paths included in save-data backups                               |

Ownership, memberships, Spark placement, resource budgets, host port assignments,
schedules, and backup storage/retention are server settings, not Ember properties.
Game-state publishing requires a compatible [game integration](/Campfire/game-integrations/),
not an additional Ember schema field.

## Identity and metadata

```json
{
  "schema": 1,
  "name": "My game",
  "description": "Dedicated server definition",
  "artUrl": "https://example.com/game.png",
  "updateUrl": "https://example.com/ember.json"
}
```

| Property      | Rules                                                                                                                   |
| ------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `name`        | Nonblank, at most 120 bytes                                                                                             |
| `description` | Descriptive text; does not affect execution                                                                             |
| `artUrl`      | HTTPS URL or absolute panel asset path, at most 2,048 bytes; no embedded credentials, whitespace, or control characters |
| `updateUrl`   | Public HTTPS definition source used by the update workflow                                                              |
| `id`          | Assigned when importing; omit when authoring a new definition                                                           |
| `revision`    | Computed by Campfire from definition content; omit when authoring                                                       |

Converted definitions may contain `origin`:

```json
{
  "origin": {
    "format": "PTDL_v2",
    "author": "Example author",
    "update_url": "https://example.com/egg.json",
    "warnings": ["Review the imported port mappings."]
  }
}
```

`format`, `author`, and `update_url` are strings; `warnings` is an array of strings.
This is provenance, not runtime configuration. The update workflow prefers
`updateUrl`, falling back to `origin.update_url`. Importing from a URL records that
source as the update URL. See [definition updates](/Campfire/embers/#definition-updates).

## Images and startup

```json
{
  "images": {
    "Java 21": "eclipse-temurin:21-jre",
    "Java 25": "eclipse-temurin:25-jre"
  },
  "startup": "java -Xmx{{SERVER_MEMORY}}M -jar {{SERVER_JAR}} nogui",
  "startup_shell": "sh",
  "work_dir": "/data",
  "user": "1000:1000"
}
```

| Property        | Rules                                                                                                               |
| --------------- | ------------------------------------------------------------------------------------------------------------------- |
| `images`        | At least one label-to-image entry; references must be nonempty, contain no whitespace/nulls, and not begin with `-` |
| `startup`       | Command template; referenced variables must be declared or built in                                                 |
| `startup_shell` | Shell name only: `sh`, `bash`, or `ash`; must exist in the selected image                                           |
| `work_dir`      | Absolute container path; no comma, newline, or null character                                                       |
| `user`          | Positive numeric UID, optionally followed by a numeric GID; root UID is not allowed                                 |

The example requires a `SERVER_JAR` variable. The selected image must contain the
runtime and shell, and the game files must be installed separately.

For multiple choices, replace `startup` with `startup_commands`:

```json
{
  "startup_commands": {
    "Normal": "java -jar {{SERVER_JAR}} nogui",
    "Diagnostics": "java -verbose:gc -jar {{SERVER_JAR}} nogui"
  }
}
```

Use at most 32 named commands. Names must be nonblank, at most 120 bytes, and have
no line breaks or nulls; commands must be nonblank. Servers select a command by
name; a single choice can be selected automatically. A nonempty `startup` and
`startup_commands` cannot coexist. Server-specific overrides are configured under
[Servers](/Campfire/servers/#edit-a-server), not in the shared definition.

### Placeholders

Use `{{NAME}}` for a declared variable or built-in environment value. Unknown
startup placeholders are rejected. Variable contents remain literal values during
startup expansion rather than becoming shell syntax.

Campfire supplies these reserved environment values as strings. Do not redeclare
their names or `STARTUP` as Ember variables.

| Built-in         | Meaning and units                                                                                                                                |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `SERVER_PORT`    | First mapped container port, not the public host port. With no mappings, falls back to the Ember's first required port; empty if neither exists. |
| `SERVER_IP`      | Container listen address, `0.0.0.0`; not the Spark's public game address.                                                                        |
| `SERVER_MEMORY`  | Configured memory limit in MiB, without a suffix; for example, `2048`.                                                                           |
| `SERVER_CPU`     | Configured CPU allocation as a percentage of one core: `100` is one core and `250` is 2.5 cores.                                                 |
| `SERVER_THREADS` | Empty; Campfire does not assign a CPU-thread list.                                                                                               |
| `SERVER_DISK`    | Configured game-data disk limit in MiB, without a suffix; `0` means no configured limit. This is not free disk space.                            |
| `SERVER_SWAP`    | `0`; additional container swap is disabled.                                                                                                      |

For example, `-Xmx{{SERVER_MEMORY}}M` appends the unit expected by Java. Resource
values describe configuration, not live CPU or memory usage.

## Installer

```json
{
  "installer": {
    "image": "alpine:3.22",
    "entrypoint": "/bin/sh",
    "script": "#!/bin/sh\nset -eu\nmkdir -p /mnt/server/world\n"
  }
}
```

| Property     | Type and rules                                             |
| ------------ | ---------------------------------------------------------- |
| `image`      | Container image reference, validated like a runtime image  |
| `entrypoint` | `sh`, `bash`, `ash`, `/bin/sh`, `/bin/bash`, or `/bin/ash` |
| `script`     | Shell script text executed by the chosen entrypoint        |

The installer uses its own image, independently of the runtime image. Its shell
must exist in that image. Game files are installed into `/mnt/server`. Omitting
`installer` means the definition supplies no installation script; importing an
Ember never runs installation or accepts a game's EULA. Treat scripts as executable
code and use trusted sources.

## Variables and validation

```json
{
  "variables": [
    {
      "name": "SERVER_JAR",
      "default": "server.jar",
      "required": true,
      "editable": true,
      "hidden": false,
      "pattern": "^[A-Za-z0-9_.-]+\\.jar$",
      "validation": { "type": "string", "min_length": 1, "max_length": 128 }
    },
    {
      "name": "MAX_PLAYERS",
      "default": "20",
      "required": true,
      "editable": true,
      "validation": { "type": "integer", "min": 1, "max": 100 }
    },
    {
      "name": "ENABLE_PVP",
      "default": "0",
      "editable": true,
      "validation": { "type": "boolean" }
    }
  ]
}
```

| Variable property | Type    | Rules / omitted value                                                                    |
| ----------------- | ------- | ---------------------------------------------------------------------------------------- |
| `name`            | string  | Unique, matching `^[A-Z][A-Z0-9_]*$`; reserved names are forbidden                       |
| `default`         | string  | Default value, including numeric and boolean values; empty if omitted                    |
| `required`        | boolean | Reject empty values; defaults to `false`                                                 |
| `editable`        | boolean | Allow per-server edits; defaults to `false`, fixing the value to its default             |
| `hidden`          | boolean | Hide from ordinary server forms; defaults to `false`                                     |
| `pattern`         | string  | Optional Go/RE2-compatible regex, at most 512 bytes; use anchors for a whole-value match |
| `validation`      | object  | Optional typed constraints below                                                         |

Values are strings in the environment, even when validation checks numeric or
boolean content. Values must be valid UTF-8, at most 8,192 bytes, with no nulls.
Hidden values are still supplied to the game and can be included in portable
backups; hiding a field is not encryption.

| Validation property        | Type             | Meaning                                                                               |
| -------------------------- | ---------------- | ------------------------------------------------------------------------------------- |
| `type`                     | string           | Required within `validation`: `string`, `integer`, `numeric`, or `boolean`            |
| `format`                   | string           | Optional `url`, valid only for string variables                                       |
| `nullable`                 | boolean          | Allow an empty, non-required value to bypass pattern/type checks; defaults to `false` |
| `min`, `max`               | number           | Numeric bounds for `integer` or `numeric`                                             |
| `min_length`, `max_length` | integer          | String-length bounds measured in Unicode characters                                   |
| `choices`                  | array of strings | Exact allowed values; empty/omitted means unrestricted                                |

Boolean values are the strings `"0"` and `"1"`, shown as toggles in the UI.
Integer values must fit a signed 64-bit integer. URL values must be absolute URLs
with a host; empty optional URLs remain allowed subject to other constraints.
Bounds must be consistent, and `required` takes precedence over `nullable`.

## Required ports

```json
{
  "required_ports": [
    {
      "container": 25565,
      "protocol": "tcp",
      "description": "Game connections"
    },
    { "container": 25565, "protocol": "udp", "description": "Query traffic" }
  ]
}
```

Each entry specifies a `container` integer from 1–65535, a `protocol` of `tcp` or
`udp`, and an optional `description` of at most 80 bytes without control
characters. Declare each protocol separately when both are needed.

These are container requirements. Each server reserves host ports from its Spark's
allocations; the definition neither opens firewalls nor configures forwarding.

## Configuration files

```json
{
  "configs": [
    {
      "path": "server.properties",
      "parser": "properties",
      "create": true,
      "values": {
        "server-port": "{{SERVER_PORT}}",
        "max-players": "{{MAX_PLAYERS}}"
      }
    },
    {
      "path": "settings.json",
      "parser": "json",
      "values": { "network.port": "{{SERVER_PORT}}" }
    }
  ]
}
```

| Property | Type    | Meaning                                                                              |
| -------- | ------- | ------------------------------------------------------------------------------------ |
| `path`   | string  | Contained relative file path; no absolute path, `..` component, backslash, or null   |
| `parser` | string  | One of the parsers below                                                             |
| `values` | object  | Replacement selectors mapped to strings, optionally containing variable placeholders |
| `create` | boolean | Create a missing file and parent directories; defaults to `false`                    |

| Parser         | Selector syntax                                                                                                            |
| -------------- | -------------------------------------------------------------------------------------------------------------------------- |
| `properties`   | Property name, such as `server-port`                                                                                       |
| `json`, `yaml` | Dot-separated keys, numeric array indices, and whole-component `*` wildcards, such as `servers.0.port` or `servers.*.port` |
| `ini`          | `key`, `section.key`, or `[section.with.dots].key`                                                                         |
| `file`         | Line prefix to match; the replacement supplies the line text                                                               |

Selectors must be nonempty, at most 1,024 bytes, and contain no control characters.
JSON/YAML selectors have at most 32 components and do not support bracket-array
syntax or partial-component wildcards. Properties replacements cannot contain
line breaks; INI and plain-file replacements must be single lines without nulls.
Parsing and rewriting can normalize formatting.

### Configuration value types

JSON/YAML replacements preserve an existing string's type, even when the text
looks numeric or boolean. Existing numbers and booleans keep their type when the
replacement converts successfully; otherwise the replacement becomes text.

For missing keys, `true`/`false` become booleans and valid JSON numbers become
numbers. Other values remain strings, including leading-zero values such as
`001`. Large integer literals retain precision. Wildcards evaluate each matched
value separately. To force numeric-looking text to stay a string, have the
installer create that key as a string first.

## Readiness and shutdown

```json
{
  "readiness": {
    "any": [
      { "value": "Server ready", "ignore_case": true },
      { "value": "Listening on port [0-9]+", "regex": true }
    ]
  },
  "stop": "stop"
}
```

| Property              | Rules                                                                       |
| --------------------- | --------------------------------------------------------------------------- |
| `ready`               | Simple text marker; omit when using `readiness`                             |
| `readiness.any`       | 1–16 alternative matchers; any match signals readiness                      |
| Matcher `value`       | 1–512 bytes; no variable placeholders                                       |
| Matcher `regex`       | Treat value as a RE2-compatible regex; defaults to `false`                  |
| Matcher `ignore_case` | Case-insensitive matching; defaults to `false`                              |
| `stop`                | One line, at most 1,024 bytes, with no null; supports variable placeholders |

Literal matchers use substring matching, and terminal color codes are ignored.
Use either `"ready": "Server ready"` or the structured `readiness` object, not both.
Readiness recognizes log output; it is not a game-port health probe.

A `stop` value such as `stop` or `quit` is a console command. Signal notation starts
with `^`: `^C` means SIGINT, and `^SIGTERM` or `^TERM` select SIGTERM. Supported
Linux signal names are validated; arbitrary caret-prefixed commands are rejected.
Omitting `stop` uses the image's default stop behavior.

## Actionable console errors

```json
{
  "log_alerts": [
    {
      "title": "Minecraft EULA requires acceptance",
      "message": "Read the EULA. If you agree, set eula=true in eula.txt and restart.",
      "severity": "error",
      "phase": "runtime",
      "any": [
        { "value": "You need to agree to the EULA", "ignore_case": true }
      ],
      "action": { "type": "open_file", "path": "eula.txt" }
    }
  ]
}
```

| Rule property | Rules                                                              |
| ------------- | ------------------------------------------------------------------ |
| `title`       | Nonblank, 1–120 bytes, no control characters                       |
| `message`     | Nonblank guidance, 1–2,048 bytes, no nulls                         |
| `severity`    | `error` or `warning`                                               |
| `phase`       | `runtime` or `installation`; omitted/empty monitors both           |
| `any`         | 1–16 alternative matchers with `value`, `regex`, and `ignore_case` |
| `action`      | Optional navigation action below                                   |

Alert matcher values are 1–512 bytes on one line without nulls. They use the same
literal/RE2 and case rules as readiness. At most 32 alerts are supported; matching
conditions must be unique. Rules need no IDs: changing a title, guidance, or order
does not reset deduplication or dismissal.

| Action               | Fields                                                                                                      |
| -------------------- | ----------------------------------------------------------------------------------------------------------- |
| Open a game file     | `{"type":"open_file","path":"eula.txt"}`; contained relative path, at most 512 bytes, no control characters |
| Open server settings | `{"type":"server_settings"}`; no path                                                                       |

Actions navigate to permission-checked interfaces; they never edit files or execute
commands. Detection continues without an open browser. See
[responding to console alerts](/Campfire/servers/#respond-to-console-alerts).

## Declaring save data

```json
{
  "savedata_paths": [
    { "path": "world", "required": true },
    { "path": "server.properties", "required": true },
    { "path": "plugins/config", "required": false }
  ]
}
```

| Property   | Rules                                                                                                         |
| ---------- | ------------------------------------------------------------------------------------------------------------- |
| `path`     | Literal file or directory path relative to the game-data root, independent of `work_dir`; at most 1,024 bytes |
| `required` | Boolean, defaults to `false`; missing required paths fail backup creation                                     |

Use at most 64 distinct, non-overlapping paths. Directories include their contents
recursively. Absolute paths, `..`, globs, shell expansion, symbolic links, special
files, the data root itself, and `.campfire-backup.json` are not allowed.

Missing optional paths are recorded; restoring such a snapshot removes a path
created there afterward. At least one selected path must exist. Include saves,
player records, configuration, and custom content needed for recovery, not binaries
the installer can recreate.

This declaration enables **Save data only** backups. Without it, only full backups
are available. Changes apply to new requests immediately, independently of restart
or server-value review. See [Game backups](/Campfire/server-backups/) for restoration.
