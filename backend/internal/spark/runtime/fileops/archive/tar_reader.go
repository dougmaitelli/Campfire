package archive

import (
	"archive/tar"
	"context"
	"io"

	"campfire/internal/shared/model"
	"campfire/internal/shared/workprogress"
)

// TarReader measures encoded input and consumed member payload separately.
// Callers validate paths and bound and drain Trailing to validate codec footers.
type TarReader struct {
	archive  *tar.Reader
	decoded  io.ReadCloser
	payload  io.Reader
	progress *workprogress.Meter
}

// NewTarGzipReader preserves the fixed portable-backup codec and header API.
// Zero inputSize denotes an unknown total.
func NewTarGzipReader(ctx context.Context, input io.Reader, inputSize int64) (*TarReader, error) {
	return newTarReader(ctx, input, inputSize, model.ArchiveTarGzip)
}

func newTarReader(ctx context.Context, input io.Reader, inputSize int64, format model.ArchiveFormat) (*TarReader, error) {
	meter := workprogress.New(ctx, workprogress.Extracting, inputSize)

	decoded, err := tarDecoder(meter.Reader(input), format)
	if err != nil {
		meter.Flush()
		return nil, normalizeDecoderError(err)
	}

	decoded = decoderReader{decoded}
	archive := tar.NewReader(decoded)

	return &TarReader{archive: archive, decoded: decoded, payload: meter.OutputReader(archive), progress: meter}, nil
}

// Next counts a header without validating its path or entry type.
func (r *TarReader) Next() (*tar.Header, error) {
	header, err := r.archive.Next()
	if err == nil {
		r.progress.Entry()
	}

	return header, err
}
func (r *TarReader) Read(p []byte) (int, error) { return r.payload.Read(p) }

// Trailing exposes the decoder for caller-bounded padding and footer validation.
func (r *TarReader) Trailing() io.Reader { return r.decoded }

// Close releases codec resources and flushes measurements; it does not validate EOF.
func (r *TarReader) Close() error {
	err := r.decoded.Close()
	r.progress.Flush()

	return err
}
