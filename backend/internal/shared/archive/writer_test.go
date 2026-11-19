package archive

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"testing"
)

type closedOutput struct{}

func (closedOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestWriterContractIsIndependentOfFormat(t *testing.T) {
	for _, name := range []string{"archive.zip", "archive.tar.gz", "archive.TGZ"} {
		t.Run(name, func(t *testing.T) {
			writer, err := NewWriter(t.Context(), closedOutput{}, name)
			if err != nil {
				t.Fatal(err)
			}

			if err := writer.Close(); err == nil {
				t.Fatal("buffered output failure lost")
			}

			writer, err = NewWriter(t.Context(), io.Discard, name)
			if err != nil {
				t.Fatal(err)
			}

			if err := writer.WriteHeader(Entry{Name: "file", Mode: 0600, Size: 3}); err != nil {
				t.Fatal(err)
			}

			if _, err := writer.Write([]byte("ok")); err != nil {
				t.Fatal(err)
			}

			if err := writer.WriteHeader(Entry{Name: "next"}); err == nil {
				t.Fatal("incomplete entry accepted")
			}

			if err := writer.Close(); err == nil {
				t.Fatal("incomplete entry closed successfully")
			}

			for _, entry := range []Entry{
				{Name: "invalid-size", Size: -1, Mode: 0600},
				{Name: "link", Mode: fs.ModeSymlink | 0600},
				{Name: "pipe", Mode: fs.ModeNamedPipe | 0600},
				{Name: "directory-with-payload", Size: 1, Mode: fs.ModeDir | 0700},
			} {
				writer, err := NewWriter(t.Context(), io.Discard, name)
				if err != nil {
					t.Fatal(err)
				}

				if err := writer.WriteHeader(entry); err == nil {
					t.Fatal("invalid member accepted", entry)
				}

				_ = writer.Close()
			}
		})
	}
}

func TestWriterDispatchRejectsUnsupportedNamesAndCancelledStarts(t *testing.T) {
	for _, name := range []string{"archive.rar", "archive.7z", "archive.tar.xz", "archive.zip.001", "archive"} {
		if _, err := NewWriter(t.Context(), io.Discard, name); err == nil {
			t.Fatal("unsupported writer selected", name)
		}
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := NewWriter(ctx, io.Discard, "archive.zip"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestZIPWriterRejectsOversizedPayload(t *testing.T) {
	writer, err := NewWriter(t.Context(), io.Discard, "archive.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()

	if err := writer.WriteHeader(Entry{Name: "file", Mode: 0600, Size: 3}); err != nil {
		t.Fatal(err)
	}

	if _, err := writer.Write([]byte("oversize")); err == nil {
		t.Fatal("oversized payload accepted")
	}
}
