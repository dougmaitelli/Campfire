// Package permissions defines server actions and their permission dependencies.
package permissions

import "slices"

// Definition describes a grant shown by role and token editors.
type Definition struct {
	ID       string   `json:"id"`
	Group    string   `json:"group"`
	Label    string   `json:"label"`
	Requires []string `json:"requires"`
}

var server = []Definition{
	{ServerView, "Server", "View overview, status, and metrics", []string{}},
	{ServerConsoleRead, "Console", "Read logs and live console output", []string{ServerView}},
	{ServerOperate, "Server", "Install, start, stop, restart, and kill; includes console reading", []string{ServerConsoleRead}},
	{ServerConsoleWrite, "Console", "Send console commands", []string{ServerOperate}},
	{ServerConfigure, "Server", "Edit configuration, resource reservations, and backup policy", []string{ServerOperate}},
	{ServerReinstall, "Server", "Update or reinstall game files", []string{ServerOperate}},
	{ServerFilesRead, "Files", "Browse, read, and download files", []string{ServerOperate}},
	{ServerFilesWrite, "Files", "Create, upload, edit, move, archive, extract, and change file permissions", []string{ServerFilesRead}},
	{ServerFilesDelete, "Files", "Delete files and folders", []string{ServerFilesWrite}},
	{ServerBackupsRead, "Backups", "View backup lists and metadata", []string{ServerOperate}},
	{ServerBackupsCreate, "Backups", "Create backups", []string{ServerBackupsRead}},
	{ServerBackupsRestore, "Backups", "Restore game files from a backup", []string{ServerBackupsRead}},
	{ServerBackupsDownload, "Backups", "Download archives containing server data", []string{ServerBackupsRead}},
	{ServerBackupsDelete, "Backups", "Delete backups", []string{ServerBackupsRead}},
	{ServerSchedulesRead, "Schedules", "View schedules and execution history", []string{ServerOperate}},
	{ServerSchedulesManage, "Schedules", "Create, edit, enable, and remove schedules", []string{ServerSchedulesRead}},
	{ServerSchedulesRun, "Schedules", "Run schedules and retry their executions", []string{ServerSchedulesRead}},
	{ServerDelete, "Server", "Delete the server and its game data", []string{ServerOperate}},
}

// Catalog returns a private copy so consumers cannot change authorization rules.
func Catalog() []Definition {
	result := slices.Clone(server)
	for i := range result {
		result[i].Requires = slices.Clone(result[i].Requires)
	}

	return result
}

// All lists the grants available to server owners.
func All() []string {
	result := make([]string, 0, len(server))
	for _, item := range server {
		result = append(result, item.ID)
	}

	return result
}

// Valid reports whether an identifier is a supported server grant.
func Valid(action string) bool { return slices.Contains(All(), action) }

// Expand adds prerequisite grants, preserving other validated scope names.
func Expand(actions []string) []string {
	result := slices.Clone(actions)
	for {
		before := len(result)
		for _, item := range server {
			if !slices.Contains(result, item.ID) {
				continue
			}

			for _, dependency := range item.Requires {
				if !slices.Contains(result, dependency) {
					result = append(result, dependency)
				}
			}
		}

		if len(result) == before {
			break
		}
	}

	slices.Sort(result)

	return slices.Compact(result)
}

// Restrict removes incomplete grants after intersecting user access and token scopes.
// It never restores a prerequisite excluded by a token.
func Restrict(actions []string) []string {
	result := slices.Clone(actions)
	for {
		before := len(result)
		for _, item := range server {
			if !slices.Contains(result, item.ID) {
				continue
			}

			for _, dependency := range item.Requires {
				if !slices.Contains(result, dependency) {
					result = slices.DeleteFunc(result, func(value string) bool { return value == item.ID })
					break
				}
			}
		}

		if len(result) == before {
			return result
		}
	}
}

// Legacy converts the old broad grants once during database migration.
func Legacy(actions []string) []string {
	result := slices.DeleteFunc(slices.Clone(actions), func(value string) bool { return value == LegacyServerShare })
	if slices.Contains(actions, ServerOperate) {
		result = append(result, ServerConsoleWrite, ServerFilesDelete, ServerBackupsCreate, ServerBackupsDownload, ServerBackupsRestore, ServerSchedulesRead)
	}

	if slices.Contains(actions, ServerConfigure) {
		result = append(result, ServerBackupsDelete)
		if slices.Contains(actions, ServerOperate) {
			result = append(result, ServerReinstall, ServerSchedulesManage, ServerSchedulesRun)
		}
	}

	return Expand(result)
}

// File maps public HTTP and SFTP operations to the same grant.
func File(operation string) string {
	switch operation {
	case "list", "stat", "read", "read.chunk":
		return ServerFilesRead
	case "write", "mkdir", "rename", "chmod", "compress", "extract", "upload.begin", "upload.chunk", "upload.commit", "upload.cancel":
		return ServerFilesWrite
	case "delete":
		return ServerFilesDelete
	default:
		return ""
	}
}

// Action maps runtime operations, including scheduled operations, to their grant.
func Action(action string) string {
	switch action {
	case "install", "start", "stop", "restart", "kill":
		return ServerOperate
	case "update", "reinstall":
		return ServerReinstall
	case "backup":
		return ServerBackupsCreate
	case "restore":
		return ServerBackupsRestore
	case "command":
		return ServerConsoleWrite
	default:
		return ""
	}
}
