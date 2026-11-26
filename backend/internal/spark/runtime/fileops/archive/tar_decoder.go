package archive

import (
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"io"

	"campfire/internal/shared/model"

	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
	"github.com/xi2/xz"
)

const decoderMemoryLimit = 128 << 20

// Decoder errors distinguish resource limits and unsupported options from corruption.
var (
	errDecoderMemory  = errors.New("archive decoder memory exceeds 128 MiB")
	errDecoderOptions = errors.New("unsupported archive compression options")
)

func tarDecoder(input io.Reader, format model.ArchiveFormat) (io.ReadCloser, error) {
	switch format {
	case model.ArchiveTar:
		return io.NopCloser(input), nil
	case model.ArchiveTarGzip:
		return gzip.NewReader(input)
	case model.ArchiveTarBzip2:
		return io.NopCloser(bzip2.NewReader(input)), nil
	case model.ArchiveTarXZ:
		// xi2 enforces a maximum (not the minimum DictCap used by ulikunitz/xz).
		reader, err := xz.NewReader(input, decoderMemoryLimit)
		if err != nil {
			return nil, err
		}

		return io.NopCloser(reader), nil
	case model.ArchiveTarLzip:
		reader, err := newLzipReader(input)
		if err != nil {
			return nil, err
		}

		return io.NopCloser(reader), nil
	case model.ArchiveTarLZ4:
		return io.NopCloser(lz4.NewReader(input)), nil
	case model.ArchiveTarZstd:
		reader, err := zstd.NewReader(input, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(decoderMemoryLimit), zstd.WithDecoderMaxWindow(decoderMemoryLimit))
		if err != nil {
			return nil, err
		}

		return reader.IOReadCloser(), nil
	default:
		return nil, errors.New("unsupported tar compression")
	}
}

type decoderReader struct{ io.ReadCloser }

func (r decoderReader) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	return n, normalizeDecoderError(err)
}

func normalizeDecoderError(err error) error {
	if errors.Is(err, xz.ErrOptions) || errors.Is(err, xz.ErrUnsupportedCheck) || errors.Is(err, zstd.ErrUnknownDictionary) || errors.Is(err, lz4.ErrInvalidFrameDescriptor) {
		return errDecoderOptions
	}

	if errors.Is(err, xz.ErrMemlimit) || errors.Is(err, zstd.ErrDecoderSizeExceeded) || errors.Is(err, zstd.ErrWindowSizeExceeded) {
		return errDecoderMemory
	}

	return err
}
