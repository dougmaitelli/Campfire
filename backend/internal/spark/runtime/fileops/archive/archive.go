// Package archive dispatches Spark's server-file extraction codecs. Filesystem
// access, path policy, conflicts, staging and publication belong to fileops.
package archive

import (
	"context"
	"io"

	sharedarchive "campfire/internal/shared/archive"
	"campfire/internal/shared/model"
)

// Visit receives one streamed member. Consume its declared payload, including
// skipped files, so the codec can validate checksums before advancing.
type Visit func(sharedarchive.Entry, io.Reader) error

// ReadLimits bounds input, expanded member sizes and metadata entry counts.
// Filesystem allocation and conflict handling remain caller policy.
type ReadLimits struct {
	MaxEntries int
	MaxBytes   int64
}

type reader func(context.Context, io.ReaderAt, int64, ReadLimits, Visit) error

func readerForName(name string) (reader, error) {
	format := model.ArchiveFormatForName(name)
	switch {
	case format == model.ArchiveZIP:
		return readZIP, nil
	case format == model.ArchiveRAR:
		return readRAR, nil
	case format.IsTar():
		return tarCodec{format: format}.read, nil
	default:
		return nil, invalidArchive("Unsupported archive format. Extract supports ZIP, single-volume unencrypted RAR, and tar (plain, gzip, xz, lzip, bzip2, lz4 or zstd). 7z is not supported.")
	}
}

// Unarchive selects a decoder by source name and streams normalized members.
// Late checksum/decode errors do not roll back previously visited members.
func Unarchive(ctx context.Context, input io.ReaderAt, size int64, name string, limits ReadLimits, visit Visit) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if limits.MaxEntries <= 0 || limits.MaxBytes <= 0 || size < 0 {
		return invalidArchive("Invalid archive read limits or size.")
	}

	if size > limits.MaxBytes {
		return archiveLimit("Archive exceeds its input size limit.")
	}

	read, err := readerForName(name)
	if err != nil {
		return err
	}

	var (
		count    int
		expanded int64
	)

	boundedVisit := func(entry sharedarchive.Entry, payload io.Reader) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		count++
		if count > limits.MaxEntries || entry.Size < 0 || entry.Size > limits.MaxBytes-expanded {
			return archiveLimit("Archive exceeds its expansion size or entry limit. Completed entries were kept.")
		}

		if !entry.Mode.IsRegular() && !entry.Mode.IsDir() {
			return invalidArchive("Archives cannot contain links or special files.")
		}

		expanded += entry.Size

		return visit(entry, contextReader{ctx, payload})
	}

	return read(ctx, contextReaderAt{ctx, input}, size, limits, boundedVisit)
}

func invalidArchive(message string) error {
	return &model.FileError{Code: model.FileInvalid, Message: message}
}
func archiveLimit(message string) error {
	return &model.FileError{Code: model.FileLimitExceeded, Message: message}
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

type contextReaderAt struct {
	ctx    context.Context
	reader io.ReaderAt
}

func (r contextReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	return r.reader.ReadAt(p, offset)
}
