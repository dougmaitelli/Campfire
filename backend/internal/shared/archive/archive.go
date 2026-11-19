package archive

import (
	"context"
	"io"
	"io/fs"
	"time"

	"campfire/internal/shared/model"
)

// Entry is format-neutral member metadata, without filesystem ownership or
// archive-specific headers. Codecs translate it at their own boundary.
type Entry struct {
	Name     string
	Size     int64
	Mode     fs.FileMode
	Modified time.Time
}

// Writer encodes members without owning traversal, staging or publication.
// WriteHeader starts a member whose payload must be written before the next
// member or Close. A successful Close is not a filesystem commit.
type Writer interface {
	io.WriteCloser
	WriteHeader(Entry) error
}

// NewWriter selects a server-file encoder by destination name. The destination
// itself is never opened or published here. Spark and the panel mock share it.
func NewWriter(ctx context.Context, output io.Writer, name string) (Writer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	switch model.ArchiveFormatForName(name) {
	case model.ArchiveTarGzip:
		return &tarFileWriter{NewTarGzipWriter(ctx, output)}, nil
	case model.ArchiveZIP:
		return newZIPWriter(ctx, output), nil
	default:
		return nil, invalidArchive("Compressed archives must end in .tar.gz, .tgz or .zip.")
	}
}

func validateWriteEntry(entry Entry) error {
	if entry.Size < 0 || (!entry.Mode.IsRegular() && !entry.Mode.IsDir()) || (entry.Mode.IsDir() && entry.Size != 0) {
		return invalidArchive("Archive entries must be regular files or directories with valid sizes.")
	}

	return nil
}

func invalidArchive(message string) error {
	return &model.FileError{Code: model.FileInvalid, Message: message}
}
