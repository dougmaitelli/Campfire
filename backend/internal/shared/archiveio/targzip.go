// Package archiveio owns streaming archive codecs and their measurements.
// Path validation, limits, conflict handling, and publication remain the caller's
// policy. A successful Close is never an operation commit.
package archiveio

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

// TarGzipReader reports compressed input and decoded payload consumed by callers.
// Consumers must validate members and drain a bounded amount of trailing data to
// validate the gzip footer before treating an archive as valid.
type TarGzipReader struct {
	archive    *tar.Reader
	compressed *gzip.Reader
	payload    io.Reader
	progress   *workprogress.Meter
}

// NewTarGzipReader starts decoding; zero inputSize denotes an unknown total.
func NewTarGzipReader(ctx context.Context, input io.Reader, inputSize int64) (*TarGzipReader, error) {
	meter := workprogress.New(ctx, workprogress.Extracting, inputSize)

	compressed, err := gzip.NewReader(meter.Reader(input))
	if err != nil {
		meter.Flush()
		return nil, err
	}

	archive := tar.NewReader(compressed)

	return &TarGzipReader{archive: archive, compressed: compressed, payload: meter.OutputReader(archive), progress: meter}, nil
}

// Next reads and counts the next archive header, without validating its path.
func (r *TarGzipReader) Next() (*tar.Header, error) {
	header, err := r.archive.Next()
	if err == nil {
		r.progress.Entry()
	}

	return header, err
}
func (r *TarGzipReader) Read(p []byte) (int, error) { return r.payload.Read(p) }

// Trailing returns the decoder after tar EOF for caller-bounded footer checks.
func (r *TarGzipReader) Trailing() io.Reader { return r.compressed }

// Close releases the decoder and reports counters; it does not validate EOF.
func (r *TarGzipReader) Close() error {
	err := r.compressed.Close()
	r.progress.Flush()

	return err
}
