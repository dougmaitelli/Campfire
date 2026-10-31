package model

import (
	"strings"
	"time"
)

// FileOperation identifies a file-management or backup-file operation.
type FileOperation string

// File operations keep their JSON values across panel and Spark transports.
const (
	FileOperationList         FileOperation = "list"
	FileOperationRead         FileOperation = "read"
	FileOperationWrite        FileOperation = "write"
	FileOperationMkdir        FileOperation = "mkdir"
	FileOperationRename       FileOperation = "rename"
	FileOperationDelete       FileOperation = "delete"
	FileOperationChmod        FileOperation = "chmod"
	FileOperationStat         FileOperation = "stat"
	FileOperationReadChunk    FileOperation = "read.chunk"
	FileOperationUploadBegin  FileOperation = "upload.begin"
	FileOperationUploadChunk  FileOperation = "upload.chunk"
	FileOperationUploadCommit FileOperation = "upload.commit"
	FileOperationUploadCancel FileOperation = "upload.cancel"
	FileOperationCompress     FileOperation = "compress"
	FileOperationExtract      FileOperation = "extract"
	FileOperationBackupList   FileOperation = "backup.list"
	FileOperationBackupRead   FileOperation = "backup.read"
	FileOperationBackupDelete FileOperation = "backup.delete"
)

// IsBackup identifies operations routed to immutable backup storage.
func (o FileOperation) IsBackup() bool { return strings.HasPrefix(string(o), "backup.") }

// IsUpload identifies operations belonging to a chunked upload session.
func (o FileOperation) IsUpload() bool { return strings.HasPrefix(string(o), "upload.") }

// FileLimit bounds each transfer to fit the authenticated command protocol.
const FileLimit = 1 << 20

// FileTransferLimit bounds streamed files without increasing command message sizes.
const FileTransferLimit int64 = 100 << 30

// DefaultUploadLimit is the default per-file upload ceiling, independently of downloads.
const DefaultUploadLimit int64 = 1 << 30

// FileRequest describes contained game file access or immutable backup list/read access.
// Spark protocol paths are relative; HTTP translates server-data-rooted paths.
type FileRequest struct {
	Mode        uint32        `json:"mode,omitempty"`
	ServerID    string        `json:"server_id"`
	Operation   FileOperation `json:"operation"`
	Path        string        `json:"path"`
	Paths       []string      `json:"paths,omitempty"`
	Destination string        `json:"destination,omitempty"`
	Data        []byte        `json:"data,omitempty"`
	Revision    string        `json:"revision,omitempty"`
	Offset      int64         `json:"offset,omitempty"`
	Size        int64         `json:"size,omitempty"`
	UploadID    string        `json:"upload_id,omitempty"`
	UploadLimit int64         `json:"upload_limit,omitempty"`
}

// FileEntry describes a directory child without following symbolic links.
type FileEntry struct {
	BackupMode BackupMode `json:"backup_mode,omitempty"`
	Mode       uint32     `json:"mode"`
	Name       string     `json:"name"`
	Kind       string     `json:"kind"`
	Size       int64      `json:"size"`
	ModifiedAt time.Time  `json:"modified_at"`
}

// FileResult contains a listing, bounded bytes, or a successful mutation acknowledgement.
type FileResult struct {
	ServerID string      `json:"server_id"`
	Path     string      `json:"path"`
	Entries  []FileEntry `json:"entries,omitempty"`
	Data     []byte      `json:"data,omitempty"`
	Revision string      `json:"revision,omitempty"`
	Size     int64       `json:"size,omitempty"`
	UploadID string      `json:"upload_id,omitempty"`
}

// FileError is a transport-independent, safe error exposed to operators.
type FileError struct {
	Code    string
	Message string
}

func (e *FileError) Error() string { return e.Message }
