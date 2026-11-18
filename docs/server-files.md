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
open folder. Folder drops are unsupported.

When a filename already exists, or repeats an earlier name in the upload batch,
choose **Skip**, **Overwrite**, or **Rename**. Overwrite is available only for regular
files, never directories or special entries. You can apply an action to remaining
conflicts; automatic renames use unique suggested names. Renaming cannot target
another existing or queued filename. If the destination changes after it was
checked, the upload fails rather than replacing the changed file.

Closing the conflict dialog cancels the remaining queue. **Cancel upload** stops
the current file and remaining queue; completed uploads are kept. **Upload results**
reports completed, explicitly skipped, failed, and not-uploaded files, with per-file
details including replacements and renames. If the final commit response is lost,
the result is **unconfirmed**, not assumed to have failed or completed; check the
destination before retrying. A failed conflict check never falls back to a blind
overwrite.

## Archives

Use **Compress** to create a `.tar.gz` archive. **Extract** accepts `.zip`,
`.tar.gz`, `.tgz`, and `.rar`. RAR support includes single-volume, unencrypted
RAR 4 and RAR 5 archives, with decoder dictionaries up to 128 MiB. Split and
password-protected RAR archives are unsupported.

Extract into a new or existing folder. Enter `.` or `./` to use the current
folder. Existing directories merge normally, preserving unrelated contents and
permissions; their existence alone does not trigger a question. There is no
upfront plan or final apply step.

Entries are read once and written directly into the destination. Nonconflicting
entries continue while actual collisions are queued. File conflicts use the same
**Skip / Overwrite / Rename** controls as uploads. Apply-to-all is offered only
when other known conflicts remain. A file cannot replace a directory, or vice
versa: choose Skip or Rename for those collisions. A blocking directory decision
holds its archive subtree, not unrelated entries; skipping or renaming the parent
also skips or relocates that subtree.

Only entries awaiting decisions (including blocked descendants) are buffered in
an OS temporary directory **outside game data**. Extraction creates no private
staging folders or new reserved filename prefixes in the game-data volume. An
approved entry is copied to its destination; it does not require a same-filesystem
rename. Temporary conflict contents are removed on completion, cancellation,
expiry, or graceful Spark shutdown. A process crash loses the operation; leftover
OS temporary data is subject to the host's temporary-file cleanup policy.

A conflict wait expires after **15 minutes** without an accepted response. Status
polling does not extend it. Active decoding/copying is not subject to this human
response deadline. Expiry stops unresolved work and keeps files already written.
An open conflict dialog closes when the server confirms expiry; it does not need
another user decision. Browser cancellation also dismisses any pending prompt.
A browser disconnect does not restart extraction; the UI uses the operation ID to
recover responses, and an unavailable operation is never restarted automatically.
Every request checks current permissions and is bound to its initiating identity
and server. A Spark restart does not resume a partially completed operation.

Overwrite decisions are checked against the observed destination immediately
before writing, after any allocation reconciliation. If it changed, review the
actual conflict again. The source archive is protected from replacement, including
through existing hardlinks. **Stop the game first** when consistent contents matter: these checks are optimistic, not a
filesystem-wide lock against external game writes. Campfire does prevent its own
filesystem-exclusive lifecycle operations and Spark transfers from overlapping an
extraction. Cancellation releases that protection only after active writers stop.
An already-queued transfer does not block extraction status or cancellation;
starting extraction or answering conflicts remains fenced until the transfer is
resolved.
Allocation checks account for extracted/replaced files and periodically reconcile
with game-data usage instead of rescanning the entire tree for every member. Where
available, the runtime's project quota remains the hard filesystem limit.

Extraction is not transactional. Errors, cancellation, bad checksums, duplicate
normalized paths, unsafe later entries, or exhausted limits can leave completed
files **and a partially written current file**. There is no rollback. Duplicate
members stop extraction instead of silently keeping the first or last version.
Results cover entries encountered before stopping; later members may not have
been read. If a response cannot be recovered, unresolved outcomes are
**unconfirmed**; check the destination before trying again. Operation-level
cancellation is kept separately from encountered entry results, so cancelling
before the first entry does not report a finished extraction. If stopping a
running scanner cannot be confirmed, completion is unconfirmed even when all
entries observed so far were written successfully.

Conflict-driven extraction into existing directories supports at most
**1,000 expanded entries**, including implicit directories, with bounded result
sizes and a 100 GiB expansion ceiling. A fresh destination uses direct one-shot
extraction without a conflict session, retaining its **100,000-entry** limit.
Cancelling the browser request on this one-shot path stops waiting for the
response, not the already-dispatched Spark command. It therefore reports
**unconfirmed** completion; extraction may continue on Spark. Check the destination
before retrying.
Reaching a limit stops explicitly and preserves partial work rather than
truncating a plan or silently skipping entries. It requires Spark capability
`server.files.extract-conflicts.v1`; older Sparks reject it without falling back
to unchecked overwrites. Allow space on both the game-data filesystem and the OS
temporary filesystem when many entries need conflict decisions.

Extraction assigns files and folders to the server's runtime user and replaces
archive permissions for newly created entries with private, writable defaults: `0600` for files and `0700`
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
