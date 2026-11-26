package fileops

import (
	"context"
	"errors"
	"io"
	"os"

	"campfire/internal/shared/model"
	"campfire/internal/spark/runtime/fileops/archive"
)

const archiveEntryLimit = 100000

type limitedArchiveWriter struct {
	io.Writer
	written int64
}

func (w *limitedArchiveWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > model.FileTransferLimit-w.written {
		return 0, errors.New("archive exceeds 100 GiB")
	}

	n, err := w.Writer.Write(data)
	w.written += int64(n)

	return n, err
}

// ReadArchive validates the source file and delegates all codec handling.
// Late decode errors may leave already extracted entries or a partial current file.
func ReadArchive(ctx context.Context, input *os.File, source string, visit archive.Visit) error {
	info, err := input.Stat()
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return Failure(model.FileInvalid, "Select a regular archive file.")
	}

	if info.Size() > model.FileTransferLimit {
		return Failure(model.FileLimitExceeded, "Archive exceeds 100 GiB.")
	}

	return archive.Unarchive(ctx, input, info.Size(), source, archive.ReadLimits{MaxEntries: archiveEntryLimit, MaxBytes: model.FileTransferLimit}, visit)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	return r.reader.Read(p)
}
