---
title: Embers
description: Create, import, and update game definitions in the panel.
---

An Ember defines how a game is installed, configured, started, monitored, and
backed up. Multiple servers can share a definition while keeping separate values,
resources, ports, and game data. For the native format, examples, and validation
rules, see the [Ember schema reference](/Campfire/ember-schema/).

## Create or edit an Ember

Choose **Create Ember** to use the visual editor, or select an existing definition
to edit it. The steps cover basics, installation, runtime, variables, ports,
configuration files, save-data paths, and review. **JSON / YAML** accepts pasted
definitions; switching tabs preserves fields and the current step. Supported
inputs are normalized to the current model when loaded, so the editor shows the
same model that will be saved. Loading does not rewrite stored definitions.
Older definitions with declared ports adopt the first as primary during
normalization; current definitions retain an intentional **No primary port**
choice. Review the selection before saving and synchronizing servers.

In **Ports**, optionally designate one **Primary game port**. It supplies the
container-side `SERVER_PORT`; its published host mapping also supplies the port
for inherited player connection addresses. **No primary port** leaves
`SERVER_PORT` empty. Games can have no ports or several ports without a primary.

## Import a definition

| Source       | Supported versions              |
| ------------ | ------------------------------- |
| Native Ember | Schema `1`, `2`                 |
| Pterodactyl  | `PTDL_v1`, `PTDL_v2`            |
| Pelican      | `PLCN_v1`, `PLCN_v2`, `PLCN_v3` |

1. Open **Embers → Import Ember** and upload, paste, or enter a public HTTPS URL.
2. Review compatibility notes and choose a startup command if multiple are offered.
3. Resolve blocking errors and acknowledge warnings before importing.
4. Create a server, review its image, values, resources, and ports, then test
   installation and startup before relying on it for a live game.

Import saves a definition; it does not download images, install a game, start it,
or accept an EULA. Use a self-contained export: inherited configuration or installer
scripts are unsupported. Unknown or missing format versions prevent import.
Empty Egg lists exported as `[]`, `{}`, or `null` are accepted.

## Conversion behavior

- Egg boolean variables retain `"1"`/`"0"` values and display as toggles.
- Plain `url` validation is preserved. Protocol-qualified rules such as
  `url:http,https` are unsupported rather than silently discarded.
- `eula`, `java_version`, `pid_limit`, and `steam_disk_space` features add predefined
  actionable error rules. Runtime selection and EULA acceptance remain explicit
  user actions. Alert rules require Spark log-alert support.
- Eggs can include the [`savedata_paths` extension](/Campfire/ember-schema/#declaring-save-data);
  Campfire does not infer save paths from variables or scripts.
- Game backup variables configure the game's own behavior, not Campfire backup
  schedules, storage, or retention.

### Warnings and blocking errors

Warnings identify behavior to review, such as ports or panel-specific features.
Neither a warning acknowledgement nor successful conversion guarantees the image
or installer will work. Converted runtime images need Bash and must support the
startup command with their existing entrypoint.

Blocking errors indicate behavior Campfire cannot preserve: examples include
inherited definitions, external mounts, privileged installers, custom log-file
tailing, file denylists, unsupported parsers, unknown placeholders, and unsupported
validation rules or regexes. Obtain a compatible export or adapt the definition;
do not simply discard restrictions without understanding their purpose.

## Definition updates

Definitions with a public update source can be checked and reviewed from **Embers**.
Review and accept changes before replacing the local definition. Historical Ember
versions are not retained.

Servers retain saved values. Changes to those values—including newly added optional
values—require review in the server wizard. Definition-only changes synchronize
independently, and backup paths use the latest declaration. Synchronization does
not run the installer or restart games; container changes take effect on restart.
Use reinstallation when game installation changes are needed. See
[applying an Ember update](/Campfire/servers/#apply-an-ember-update).
