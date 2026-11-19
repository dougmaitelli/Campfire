package fileops

import (
	"archive/tar"
	"archive/zip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"

	"campfire/internal/shared/archiveio"
	"campfire/internal/shared/model"
	"campfire/internal/shared/workprogress"
)

const archiveEntryLimit = 100000

// ArchiveEntry receives one streamed archive member.
type ArchiveEntry func(string, int64, fs.FileMode, io.Reader) error

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

// ReadArchive streams entries once. A late decode/checksum error is allowed to
// leave earlier entries (and a partially written current file) in the destination.
func ReadArchive(ctx context.Context, input *os.File, source string, unpack ArchiveEntry) error {
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

	source = strings.ToLower(source)
	switch {
	case strings.HasSuffix(source, ".zip"):
		if err := CheckZIPDirectory(input, info.Size()); err != nil {
			return err
		}

		progress := workprogress.New(ctx, workprogress.Extracting, 0)
		defer progress.Flush()

		archive, err := zip.NewReader(progress.ReaderAt(input), info.Size())
		if err != nil {
			return err
		}

		if len(archive.File) > 100000 {
			return Failure(model.FileLimitExceeded, "Archive has too many entries.")
		}

		for _, item := range archive.File {
			if item.UncompressedSize64 > uint64(model.FileTransferLimit) {
				return Failure(model.FileLimitExceeded, "Archive expands beyond 100 GiB.")
			}

			reader, err := item.Open()
			if err != nil {
				return err
			}

			progress.Entry()

			err = unpack(item.Name, int64(item.UncompressedSize64), item.Mode(), progress.OutputReader(reader))
			if err == nil {
				var tail [1]byte

				n, end := reader.Read(tail[:])
				if n != 0 || end != io.EOF {
					err = errors.New("invalid ZIP entry checksum or length")
				}
			}

			_ = reader.Close()

			if err != nil {
				return err
			}
		}

		return nil
	case strings.HasSuffix(source, ".rar"):
		progress := workprogress.New(ctx, workprogress.Extracting, info.Size())
		defer progress.Flush()

		return ExtractRAR(progress.Reader(contextReader{ctx, input}), func(name string, size int64, mode fs.FileMode, reader io.Reader) error {
			progress.Entry()
			return unpack(name, size, mode, progress.OutputReader(reader))
		})
	case strings.HasSuffix(source, ".tar.gz"), strings.HasSuffix(source, ".tgz"):
		archive, err := archiveio.NewTarGzipReader(ctx, contextReader{ctx, input}, info.Size())
		if err != nil {
			return err
		}
		defer func() { _ = archive.Close() }()

		for {
			header, err := archive.Next()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				return err
			}

			var mode fs.FileMode

			switch header.Typeflag {
			case tar.TypeDir:
				mode = fs.ModeDir | fs.FileMode(header.Mode)&0777
			case tar.TypeReg, 0:
				mode = fs.FileMode(header.Mode) & 0777
			default:
				return Failure(model.FileInvalid, "Archives cannot contain links or special files.")
			}

			if err := unpack(header.Name, header.Size, mode, archive); err != nil {
				return err
			}
		}

		n, err := io.Copy(io.Discard, io.LimitReader(contextReader{ctx, archive.Trailing()}, model.FileLimit+1))
		if err != nil {
			return err
		}

		if n > model.FileLimit {
			return Failure(model.FileInvalid, "Unexpected trailing archive data.")
		}

		return nil
	default:
		return Failure(model.FileInvalid, "Extract supports .zip, .tar.gz, .tgz and .rar archives.")
	}
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
