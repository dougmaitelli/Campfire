package archive

import (
	"context"
	"errors"
	"io"

	"campfire/internal/shared/workprogress"

	"github.com/nwaples/rardecode/v2"
)

func readRAR(ctx context.Context, input io.ReaderAt, size int64, _ ReadLimits, visit Visit) error {
	progress := workprogress.New(ctx, workprogress.Extracting, size)
	defer progress.Flush()

	// Bound decoder memory independently of the archive's expanded size limit.
	archive, err := rardecode.NewReader(progress.Reader(io.NewSectionReader(input, 0, size)), rardecode.MaxDictionarySize(128<<20))
	if err != nil {
		return rarFailure(err)
	}

	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return rarFailure(err)
		}

		if header.Encrypted || header.HeaderEncrypted {
			return rarFailure(rardecode.ErrArchiveEncrypted)
		}

		if header.UnKnownSize || header.LinkType != 0 || header.LinkTarget != "" {
			return invalidArchive("RAR entries must have known sizes and cannot contain links.")
		}
		// Mode() only identifies symlinks; reject other Unix special types explicitly.
		unixType := header.Attributes & 0xf000
		if header.HostOS == rardecode.HostOSUnix && unixType != 0 && unixType != 0x4000 && unixType != 0x8000 {
			return invalidArchive("Archives cannot contain links or special files.")
		}

		progress.Entry()

		entry := Entry{Name: header.Name, Size: header.UnPackedSize, Mode: header.Mode(), Modified: header.ModificationTime}
		if err := visit(entry, progress.OutputReader(archive)); err != nil {
			return rarFailure(err)
		}
		// Reading past the declared size verifies the entry checksum, including empty files.
		var tail [1]byte

		n, end := archive.Read(tail[:])
		if n != 0 || !errors.Is(end, io.EOF) {
			if end != nil && !errors.Is(end, io.EOF) {
				return rarFailure(end)
			}

			return invalidArchive("Invalid RAR entry length.")
		}
	}
}

func rarFailure(err error) error {
	switch {
	case errors.Is(err, rardecode.ErrArchiveEncrypted), errors.Is(err, rardecode.ErrArchivedFileEncrypted):
		return invalidArchive("Password-protected RAR archives are not supported.")
	case errors.Is(err, rardecode.ErrMultiVolume), errors.Is(err, rardecode.ErrFileNameRequired):
		return invalidArchive("Split RAR archives are not supported. Use a single-volume archive.")
	case errors.Is(err, rardecode.ErrDictionaryTooLarge):
		return archiveLimit("RAR decoder dictionaries are limited to 128 MiB.")
	default:
		return err
	}
}
