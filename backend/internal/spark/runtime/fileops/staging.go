package fileops

import "strings"

// Existing upload/archive publication uses these names. Interactive extraction
// adds no reserved names: only waiting entries use the OS temporary directory.
const (
	ArchiveStagingPrefix = ".campfire-archive-"
	UploadStagingPrefix  = ".campfire-upload-"
)

// IsStagingName recognizes the existing upload/archive private namespace.
func IsStagingName(name string) bool {
	return strings.HasPrefix(name, ArchiveStagingPrefix) || strings.HasPrefix(name, UploadStagingPrefix)
}
