package archive

import (
	"archive/zip"
	"context"
	"errors"
	"io"

	"campfire/internal/shared/workprogress"
)

func readZIP(ctx context.Context, input io.ReaderAt, size int64, limits ReadLimits, visit Visit) error {
	if err := checkZIPDirectory(input, size, limits.MaxEntries); err != nil {
		return err
	}
	// ZIP seeks and may reread bytes, so there is no meaningful input total.
	progress := workprogress.New(ctx, workprogress.Extracting, 0)
	defer progress.Flush()

	archive, err := zip.NewReader(progress.ReaderAt(input), size)
	if err != nil {
		return err
	}

	if len(archive.File) > limits.MaxEntries {
		return archiveLimit("Archive has too many entries.")
	}

	for _, item := range archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}

		if item.Flags&0x2041 != 0 {
			return invalidArchive("Password-protected ZIP archives are not supported.")
		}

		if item.Method != zip.Store && item.Method != zip.Deflate {
			return invalidArchive("Unsupported ZIP compression method. Use stored or deflated ZIP entries.")
		}
		// Check the unsigned size before converting it into common metadata.
		if item.UncompressedSize64 > uint64(limits.MaxBytes) {
			return archiveLimit("Archive exceeds its expansion size limit.")
		}

		reader, err := item.Open()
		if err != nil {
			return err
		}

		progress.Entry()

		err = visit(Entry{Name: item.Name, Size: int64(item.UncompressedSize64), Mode: item.Mode(), Modified: item.Modified}, progress.OutputReader(reader))
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
}
