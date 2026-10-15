package model

import "time"

// FileLimit bounds each transfer to fit the authenticated command protocol.
const FileLimit = 1 << 20

// FileTransferLimit bounds streamed files without increasing command message sizes.
const FileTransferLimit int64 = 100 << 30

// DefaultUploadLimit is the default per-file upload ceiling, independently of downloads.
const DefaultUploadLimit int64 = 1 << 30

// FileRequest describes contained game file access or immutable backup list/read access.
type FileRequest struct {
	ServerID    string `json:"server_id"`
	Operation   string `json:"operation"`
	Path        string `json:"path"`
	Destination string `json:"destination,omitempty"`
	Data        []byte `json:"data,omitempty"`
	Revision    string `json:"revision,omitempty"`
	Offset      int64  `json:"offset,omitempty"`
	Size        int64  `json:"size,omitempty"`
	UploadID    string `json:"upload_id,omitempty"`
	UploadLimit int64  `json:"upload_limit,omitempty"`
}

// FileEntry describes a directory child without following symbolic links.
type FileEntry struct {
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
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
