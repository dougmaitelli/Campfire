package runtime

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestArchiveConflictNamesAndPartialExtraction(t *testing.T) {
	for _, format := range []string{"zip", "tar.gz"} {
		t.Run(format, func(t *testing.T) {
			m, d := fileTestManager(t)

			var data bytes.Buffer

			names := []string{"nested/save.txt", "./nested/save.txt"}

			if format == "zip" {
				writer := zip.NewWriter(&data)
				for _, name := range names {
					entry, err := writer.Create(name)
					if err != nil {
						t.Fatal(err)
					}

					if _, err := entry.Write([]byte("save")); err != nil {
						t.Fatal(err)
					}
				}

				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				compressed := gzip.NewWriter(&data)

				writer := tar.NewWriter(compressed)
				for _, name := range names {
					if err := writer.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: 4}); err != nil {
						t.Fatal(err)
					}

					if _, err := writer.Write([]byte("save")); err != nil {
						t.Fatal(err)
					}
				}

				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}

				if err := compressed.Close(); err != nil {
					t.Fatal(err)
				}
			}

			volume := filepath.Join(m.root(d.ServerID), "data")

			archive := "duplicate." + format
			if err := os.WriteFile(filepath.Join(volume, archive), data.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}

			_, err := m.Files(t.Context(), d, model.FileRequest{ServerID: d.ServerID, Operation: model.FileOperationExtract, Path: archive, Destination: "output"})

			var failure *model.FileError
			if !errors.As(err, &failure) || failure.Code != model.FileInvalid || !strings.Contains(failure.Message, `duplicate path "nested/save.txt"`) || !strings.Contains(failure.Message, "Completed entries were kept") {
				t.Fatalf("duplicate must identify the normalized member: %v", err)
			}

			if data, err := os.ReadFile(filepath.Join(volume, "output/nested/save.txt")); err != nil || string(data) != "save" {
				t.Fatal("completed entry was not kept", string(data), err)
			}

			stages, err := filepath.Glob(filepath.Join(volume, ".campfire-archive-*"))
			if err != nil || len(stages) != 0 {
				t.Fatal("failed extraction leaked staging", stages, err)
			}

			if err := os.Mkdir(filepath.Join(volume, "existing"), 0700); err != nil {
				t.Fatal(err)
			}

			_, err = m.Files(t.Context(), d, model.FileRequest{ServerID: d.ServerID, Operation: model.FileOperationExtract, Path: archive, Destination: "existing"})
			if !errors.As(err, &failure) || failure.Code != model.FileConflict || !strings.Contains(failure.Message, `"existing"`) || !strings.Contains(failure.Message, "not merged or overwritten") {
				t.Fatalf("occupied destination must explain the conflict: %v", err)
			}
		})
	}
}
