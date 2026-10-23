# Game files

Open a server's **Files** tab to browse its game directory. You can view and edit
text files, create files and folders, rename entries, remove files or empty
folders, and upload or download files. This view does not expose host files or
Spark's private metadata.

Browsing and downloads work while a game is running. Stop the server before
making file changes. Other active server operations can also prevent access.
Text edits check the file revision: reload a changed file before saving again.

## Transfers and archives

The browser displays the configured upload limit. Its default is 1024 MiB per
file; operators can change `CAMPFIRE_MAX_UPLOAD_MIB` on the panel. The text editor
has a separate 1 MiB limit. Uploads show progress and can be cancelled.

Use **Compress** to create a `.tar.gz` archive. **Extract** accepts `.zip`,
`.tar.gz`, and `.tgz` and requires a new destination folder. Stop the game first
and leave enough disk space for the original files and the archive or extracted
copy. Archives do not merge into an existing destination.

## SFTP

Operators can enable the panel's SFTP gateway with `CAMPFIRE_SFTP_LISTEN` and
publish that TCP port separately from HTTP. Connect to the panel's SFTP address,
use the server ID shown in **Files** as the username, and use a personal API
token as the password. Verify the panel's SSH host-key fingerprint in your client.

The account and token need `server.view` and `server.operate` access. SFTP exposes
only that server's game files. File changes still require a stopped server;
shell access, forwarding, upload resume, and in-place editing are unsupported.
