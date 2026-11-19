package archive

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"

	"campfire/internal/shared/workprogress"
)

type zipWriter struct {
	archive   *zip.Writer
	payload   io.Writer
	progress  *workprogress.Meter
	remaining int64
}

func newZIPWriter(ctx context.Context, output io.Writer) Writer {
	meter := workprogress.New(ctx, workprogress.Archiving, 0)
	return &zipWriter{archive: zip.NewWriter(meter.Writer(output)), progress: meter}
}

func (w *zipWriter) WriteHeader(entry Entry) error {
	if w.remaining != 0 {
		return io.ErrUnexpectedEOF
	}

	if err := validateWriteEntry(entry); err != nil {
		return err
	}

	item := &zip.FileHeader{Name: entry.Name, Method: zip.Deflate, UncompressedSize64: uint64(entry.Size), Modified: entry.Modified}
	item.SetMode(entry.Mode & (fs.ModeDir | fs.ModePerm))

	if entry.Mode.IsDir() {
		item.Name = strings.TrimSuffix(item.Name, "/") + "/"
		item.Method = zip.Store
	}

	payload, err := w.archive.CreateHeader(item)
	if err != nil {
		return err
	}

	w.payload = w.progress.InputWriter(payload)
	w.remaining = entry.Size
	w.progress.Entry()

	return nil
}
func (w *zipWriter) Write(p []byte) (int, error) {
	if w.payload == nil || int64(len(p)) > w.remaining {
		return 0, errors.New("ZIP payload exceeds declared entry size")
	}

	n, err := w.payload.Write(p)
	w.remaining -= int64(n)

	return n, err
}
func (w *zipWriter) Close() error {
	err := w.archive.Close()
	w.progress.Flush()

	if w.remaining != 0 {
		return errors.Join(io.ErrUnexpectedEOF, err)
	}

	return err
}
