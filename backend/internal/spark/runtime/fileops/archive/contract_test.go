package archive

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"testing"
	"time"

	sharedarchive "campfire/internal/shared/archive"
	"campfire/internal/shared/model"
	"campfire/internal/shared/workprogress"
)

func TestCommonCallersRoundTripWithoutCodecKnowledge(t *testing.T) {
	for _, name := range []string{"archive.zip", "archive.tar.gz", "archive.TGZ"} {
		t.Run(name, func(t *testing.T) {
			var (
				data               bytes.Buffer
				created, extracted workprogress.Snapshot
			)

			ctx := workprogress.WithReporter(t.Context(), func(s workprogress.Snapshot) { created = s })

			writer, err := sharedarchive.NewWriter(ctx, &data, name)
			if err != nil {
				t.Fatal(err)
			}

			modified := time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC)

			entries := []sharedarchive.Entry{
				{Name: "world/", Mode: fs.ModeDir | 0700, Modified: modified},
				{Name: "world/empty", Mode: 0600, Modified: modified},
				{Name: "world/save", Size: 12, Mode: 0600 | fs.ModeSetuid, Modified: modified},
			}
			for _, entry := range entries {
				if err := writer.WriteHeader(entry); err != nil {
					t.Fatal(err)
				}

				if entry.Size > 0 {
					if _, err := io.WriteString(writer, "saved world\n"); err != nil {
						t.Fatal(err)
					}
				}
			}

			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}

			if created.InputBytes != 12 || created.OutputBytes != int64(data.Len()) || created.Entries != 3 || created.InputTotal != 0 {
				t.Fatal(created)
			}

			ctx = workprogress.WithReporter(t.Context(), func(s workprogress.Snapshot) { extracted = s })
			index := 0
			limits := ReadLimits{MaxEntries: 100, MaxBytes: 1 << 20}

			err = Unarchive(ctx, bytes.NewReader(data.Bytes()), int64(data.Len()), name, limits, func(entry sharedarchive.Entry, reader io.Reader) error {
				if index >= len(entries) {
					t.Fatal("extra member", entry)
				}

				want := entries[index]
				if entry.Name != want.Name || entry.Size != want.Size || entry.Mode.Perm() != want.Mode.Perm() || entry.Mode.IsDir() != want.Mode.IsDir() || entry.Mode&fs.ModeSetuid != 0 || !entry.Modified.Equal(modified) {
					t.Fatal(entry, want)
				}

				body, err := io.ReadAll(reader)
				if err != nil {
					return err
				}

				if entry.Size > 0 && string(body) != "saved world\n" {
					t.Fatal(string(body))
				}

				index++

				return nil
			})
			if err != nil || index != len(entries) {
				t.Fatal(index, err)
			}

			if extracted.OutputBytes != 12 || extracted.Entries != 3 {
				t.Fatal(extracted)
			}

			stop := errors.New("visitor stopped")

			err = Unarchive(t.Context(), bytes.NewReader(data.Bytes()), int64(data.Len()), name, limits, func(sharedarchive.Entry, io.Reader) error { return stop })
			if !errors.Is(err, stop) {
				t.Fatal("visitor failure lost", err)
			}

			limits.MaxEntries = 1
			err = Unarchive(t.Context(), bytes.NewReader(data.Bytes()), int64(data.Len()), name, limits, func(_ sharedarchive.Entry, reader io.Reader) error { _, err := io.Copy(io.Discard, reader); return err })

			var failure *model.FileError
			if !errors.As(err, &failure) {
				t.Fatal("entry limit ignored", err)
			}
		})
	}
}
