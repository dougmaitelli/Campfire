package archive

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"io/fs"

	"campfire/internal/shared/model"
)

const maxTarPadding = 1 << 20

type tarCodec struct{ format model.ArchiveFormat }

func (c tarCodec) read(ctx context.Context, input io.ReaderAt, size int64, _ ReadLimits, visit Visit) (err error) {
	defer func() { err = tarFailure(err) }()

	archive, err := newTarReader(ctx, io.NewSectionReader(input, 0, size), size, c.format)
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
			return invalidArchive("Archives cannot contain links or special files.")
		}

		if err := visit(Entry{Name: header.Name, Size: header.Size, Mode: mode, Modified: header.ModTime}, archive); err != nil {
			return err
		}
	}

	n, err := io.Copy(tarPaddingWriter{}, io.LimitReader(contextReader{ctx, archive.Trailing()}, maxTarPadding+1))
	if err != nil {
		return err
	}

	if n > maxTarPadding {
		return invalidArchive("Unexpected trailing archive data.")
	}

	return nil
}

func tarFailure(err error) error {
	if errors.Is(err, errDecoderOptions) {
		return invalidArchive("Archive uses unsupported compression options. External dictionaries are not supported; lzip requires version 1.")
	}

	if errors.Is(err, errDecoderMemory) {
		return archiveLimit("Archive decoder dictionaries and windows are limited to 128 MiB.")
	}

	return err
}

// Only zero padding is valid after tar EOF. Still drain it to check codec
// footers; concatenated tar archives must not silently hide later members.
type tarPaddingWriter struct{}

func (tarPaddingWriter) Write(p []byte) (int, error) {
	for _, b := range p {
		if b != 0 {
			return 0, invalidArchive("Unexpected data after tar end. Concatenated tar archives are not supported.")
		}
	}

	return len(p), nil
}
