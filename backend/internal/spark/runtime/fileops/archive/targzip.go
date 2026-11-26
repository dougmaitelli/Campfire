package archive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"

	"campfire/internal/shared/workprogress"
)

// TarGzipWriter measures payload and compressed output separately.
type TarGzipWriter struct {
	archive    *tar.Writer
	compressed *gzip.Writer
	payload    io.Writer
	progress   *workprogress.Meter
}

// NewTarGzipWriter reports uncompressed payload input and compressed output.
// It does not scan the source twice to invent a total for mutable game data.
func NewTarGzipWriter(ctx context.Context, output io.Writer) *TarGzipWriter {
	meter := workprogress.New(ctx, workprogress.Archiving, 0)
	compressed := gzip.NewWriter(meter.Writer(output))
	archive := tar.NewWriter(compressed)

	return &TarGzipWriter{archive: archive, compressed: compressed, payload: meter.InputWriter(archive), progress: meter}
}

// WriteHeader counts an entry after its header has been accepted.
func (w *TarGzipWriter) WriteHeader(header *tar.Header) error {
	if err := w.archive.WriteHeader(header); err != nil {
		return err
	}

	w.progress.Entry()

	return nil
}
func (w *TarGzipWriter) Write(p []byte) (int, error) { return w.payload.Write(p) }

// Close flushes both encoders and reports final counters, including on error.
func (w *TarGzipWriter) Close() error {
	// Always close both layers; gzip's final buffered output must be measured.
	archiveErr := w.archive.Close()
	gzipErr := w.compressed.Close()
	w.progress.Flush()

	return errors.Join(archiveErr, gzipErr)
}
