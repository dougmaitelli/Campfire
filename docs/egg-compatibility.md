---
title: Importing game definitions
---

Campfire imports native Embers and supported Pterodactyl or Pelican Eggs.
An Egg is converted into a Campfire Ember describing the game's images,
installation, startup, variables, and configuration.

## Supported exports

| Source       | Supported versions              |
| ------------ | ------------------------------- |
| Native Ember | Schema 1                        |
| Pterodactyl  | `PTDL_v1`, `PTDL_v2`            |
| Pelican      | `PLCN_v1`, `PLCN_v2`, `PLCN_v3` |

Definitions can be JSON or YAML, up to 1 MiB. Use a self-contained export;
definitions that inherit configuration or installer scripts from another Egg
are unsupported. An unknown or missing format version prevents import.

Empty Egg lists may be exported as `[]`, `{}`, or `null`. Campfire accepts these
empty forms while still checking nonempty lists and their entries.

## Import an Egg

1. Open **Embers** and choose **Import Ember**.
2. Upload a file, paste its contents, or enter a public HTTPS URL.
3. Review the compatibility preview. If the export offers multiple startup
   commands, choose the one you want to use.
4. Resolve any blocking errors and acknowledge the warnings before importing.
5. Create a server from the Ember, then check its image, variables, game ports,
   and resource allocation before installing it.

Importing saves a definition. It does not download container images, run the
installer, start a game, or accept a game's EULA. A successful import confirms
that the definition can be converted; test installation and startup on your
Spark before relying on it for a live server.

## What to review

| Area                   | What to check                                                                                                                                       |
| ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| Images and installer   | Use images and scripts you trust. Converted runtime images need Bash and must work with the imported startup command and their existing entrypoint. |
| Variables              | Check required values and defaults. Non-editable variables remain fixed to their defaults; hidden variables do not appear in ordinary server forms. |
| Ports                  | Confirm both the game port and TCP/UDP mappings. `SERVER_PORT` refers to the container port, which may differ from the published host port.         |
| Configuration files    | Supported parsers include properties, JSON, YAML, INI, and line-prefix text replacements. Conversion may normalize file formatting.                 |
| Readiness and shutdown | Check that startup detection matches the game's logs and that its stop command or signal shuts it down as expected.                                 |
| Game requirements      | Select the correct Java or other runtime image yourself and accept any required EULA explicitly. These choices are not made automatically.          |

Boolean variables, such as `ENABLE_CROSSPLAY`, use toggles in server settings
and keep the Egg's `1` (enabled) and `0` (disabled) values.
Server details and configuration reviews display these as **Enabled** or **Disabled**.

The plain `url` variable rule is preserved as native URL validation. Values must
be absolute URLs with a host, checked using Go's URL parser at both the panel and
Spark boundaries. Empty optional values remain allowed. Protocol-qualified rules
such as `url:http,https` remain unsupported; they are not silently discarded.

Game variables do not configure Campfire's backup policy. For example, Valheim
Eggs pass `BACKUP_INTERVAL`, `BACKUP_COUNT`, `BACKUP_SHORTTIME`, and `BACKUP_LONGTIME`
to the game's own save and backup flags. These remain ordinary game variables.
Configure Campfire's backup schedules, storage, and retention separately.

## Declaring save data

Native Embers can declare `savedata_paths` to offer **Save data only** backups:

```json
"savedata_paths": [
  { "path": "world", "required": true },
  { "path": "server.properties", "required": true },
  { "path": "plugins/config", "required": false }
]
```

Each entry selects a file or an entire directory recursively. Paths are literal,
relative to the server's game-data root, independently of `work_dir`. Include the
worlds, player records, configuration, and custom content needed for recovery;
omit binaries that the installer can recreate. A required path must exist or the
backup fails. An absent optional path is recorded in the archive; restoring that
snapshot removes a subsequently created path at that location.
At least one selected path must exist for a save-data backup to succeed.

Use at most 64 distinct, non-overlapping paths of at most 1024 bytes each. Absolute
paths, `..`, glob patterns, shell expansion, symbolic links, special files, and
the reserved `.campfire-backup.json` name are rejected. The game-data root itself
cannot be selected; use a full backup for that.

Imported Eggs may carry the same optional `savedata_paths` extension. Campfire
does not infer these paths from game variables or installation scripts. Without
an explicit declaration, only full backups are available. After changing the
declaration, apply the updated Ember to the server before creating save-data
backups. See [game backups](/Campfire/server-backups/) for restore and retention
behavior.

## Warnings and blocking errors

Warnings identify behavior that needs your review, such as port interpretation
or panel-specific game features. Conversion details provide additional notes.
Neither guarantees that the game image or installation script will work.

Blocking errors mean Campfire cannot preserve required behavior. Common causes
include inherited definitions, external mounts, privileged installers, custom
log-file tailing, file denylists, unsupported configuration parsers, unknown
placeholders, and unsupported validation rules or regular expressions.

Obtain a compatible self-contained export or adapt the definition to Campfire's
supported behavior before importing again. Do not discard a restriction, such
as a file denylist, without understanding why the original Egg required it.

## Definition updates

Embers imported from a public URL can retain that URL as an update source.
The Embers page can check for updates and open the review flow for a new
definition. Review and accept changes before replacing the local Ember.
Existing servers keep their applied revision until you explicitly apply the
updated Ember to them. Applying a definition update does not run its installer;
use the server's reinstall workflow when game installation changes are needed.
