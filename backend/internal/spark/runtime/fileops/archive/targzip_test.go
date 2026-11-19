package archive

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"testing"

	sharedarchive "campfire/internal/shared/archive"
	"campfire/internal/shared/workprogress"
)

func TestTarGzipReportsPayloadAndEncodedBytesSeparately(t *testing.T) {
	payload := bytes.Repeat([]byte("saved world"), 10000)

	var (
		data     bytes.Buffer
		creation workprogress.Snapshot
	)

	ctx := workprogress.WithReporter(t.Context(), func(s workprogress.Snapshot) { creation = s })

	writer := sharedarchive.NewTarGzipWriter(ctx, &data)
	if err := writer.WriteHeader(&tar.Header{Name: "world", Mode: 0600, Size: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}

	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	if creation.InputBytes != int64(len(payload)) || creation.OutputBytes != int64(data.Len()) || creation.InputTotal != 0 || creation.Entries != 1 || creation.Phase != workprogress.Archiving {
		t.Fatal(creation)
	}

	if creation.OutputBytes >= creation.InputBytes {
		t.Fatal("fixture should compress")
	}

	var extraction workprogress.Snapshot

	ctx = workprogress.WithReporter(t.Context(), func(s workprogress.Snapshot) { extraction = s })

	reader, err := NewTarGzipReader(ctx, bytes.NewReader(data.Bytes()), int64(data.Len()))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := reader.Next(); err != nil {
		t.Fatal(err)
	}

	decoded, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(decoded, payload) {
		t.Fatal(err)
	}

	if _, err := reader.Next(); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}

	if _, err := io.Copy(io.Discard, reader.Trailing()); err != nil {
		t.Fatal(err)
	}

	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}

	if extraction.InputBytes != int64(data.Len()) || extraction.InputTotal != int64(data.Len()) || extraction.OutputBytes != int64(len(payload)) || extraction.Entries != 1 || extraction.Phase != workprogress.Extracting {
		t.Fatal(extraction)
	}
}

type closedOutput struct{}

func (closedOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestBufferedCloseAndCorruptFooterCannotBecomeSuccess(t *testing.T) {
	var last workprogress.Snapshot

	ctx := workprogress.WithReporter(t.Context(), func(s workprogress.Snapshot) { last = s })

	writer := sharedarchive.NewTarGzipWriter(ctx, closedOutput{})
	if err := writer.Close(); err == nil {
		t.Fatal("expected close failure")
	}

	if last.OutputBytes != 0 || last.Phase != workprogress.Archiving {
		t.Fatal(last)
	}

	var data bytes.Buffer

	writer = sharedarchive.NewTarGzipWriter(ctx, &data)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	damaged := bytes.Clone(data.Bytes())
	damaged[len(damaged)-8] ^= 0xff

	reader, err := NewTarGzipReader(ctx, bytes.NewReader(damaged), int64(len(damaged)))
	if err != nil {
		t.Fatal(err)
	}

	_, nextErr := reader.Next()
	_, footerErr := io.Copy(io.Discard, reader.Trailing())
	_ = reader.Close()

	if (nextErr == nil || errors.Is(nextErr, io.EOF)) && footerErr == nil {
		t.Fatal("corrupt footer accepted")
	}

	if last.Phase != workprogress.Extracting || last.InputBytes != int64(len(damaged)) {
		t.Fatal(last)
	}
}
