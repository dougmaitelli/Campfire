---
title: Game files
---

Open a server's **Files** tab to browse its game directory. You can view and edit
text files, create files and folders, rename entries, remove folders and their
contents, change permissions, and upload or download files. This view does not expose host files or
Spark's private metadata.

Browsing and downloading require `server.files.read`. Editing, uploads, moving,
archives, and chmod require `server.files.write`; deletion requires
`server.files.delete`. See the [permission hierarchy](/Campfire/authentication/#permission-hierarchy)
for prerequisite grants.

Browsing, downloads, and file changes work while a game is running, starting,
stopping, or restarting, including a force-stop. Stopping the game before changing
files is recommended to avoid conflicts with the game's own writes. Installation,
updates, reinstalls, backups/restores, deletion, and transfers can still restrict
file access. Text edits check the file revision: reload a changed file before
saving again.

## Paths and the server data root

The **Server data** breadcrumb is `/` in the file manager. This is the server's
persistent data directory, mounted at the Ember's working directory (for example,
`/home/container` or `/data`). It is separate from the container image's system
root and the Spark host filesystem.

Paths always start at this data root. For example, `/config/fabric/` is a folder
and `/config/fabric/settings.json` is a file. A move destination has the same
meaning regardless of which folder you are viewing.

## Uploads

The browser displays the configured upload limit. Its default is 1024 MiB per
file; operators can change `CAMPFIRE_MAX_UPLOAD_MIB` on the panel. The text editor
has a separate 1 MiB limit. Uploads show progress and can be cancelled.

Choose **Upload Files** or drag files onto the file browser to upload into the
open folder. Folder drops are unsupported, and existing files are not overwritten.
**Cancel upload** stops the current file and remaining queue; completed uploads
are kept.

## Archives

Use **Compress** to create a `.tar.gz` archive. **Extract** accepts `.zip`,
`.tar.gz`, `.tgz`, and `.rar` and requires a new destination folder. RAR support
includes single-volume, unencrypted RAR 4 and RAR 5 archives, with decoder
dictionaries up to 128 MiB. Split and password-protected RAR archives are unsupported.
Stop the game first to avoid concurrent changes during compression or extraction. Leave enough disk space for the original files and the archive or
extracted copy. Archives do not merge into an existing destination.

Extraction assigns files and folders to the server's runtime user and replaces
archive permissions with private, writable defaults: `0600` for files and `0700`
for directories. Executable files receive `0700`; group, other, and special
permission bits are discarded. ZIP files created on some platforms do not record
executable permissions.

## Permissions

Select files or folders and choose **Permissions** to apply an octal mode to all
selected items.
For example, use `755` for an executable startup script such as
`start_game_bepinex.sh`, or `644` for a regular configuration file. Changes affect
only the selected items. Folder contents are not changed. Spark must support the
file permissions capability.

## SFTP

Operators can enable the panel's SFTP gateway with `CAMPFIRE_SFTP_LISTEN` and
publish that TCP port separately from HTTP. Connect to the panel's SFTP address,
use the server ID shown in **Files** as the username, and use a personal API
token as the password. Verify the panel's SSH host-key fingerprint in your client.

The account and token need `server.files.read` and its prerequisites. Add
`server.files.write` for uploads and file changes, and `server.files.delete` for
deletion. SFTP exposes only that server's game files and checks current access on
each operation. Shell access, forwarding, upload resume, and in-place editing are
unsupported.
