//go:build !production

package devmock

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestMockSelectedArchiveProducesDownloadableTarGzip(t *testing.T) {
	record := runtimeRecord{Files: map[string]string{
		"config/": "", "config/a.txt": "first", "config/b.txt": "second", "config/ignored.txt": "skip",
	}}

	err := mockCompress(t.Context(), &record, model.FileRequest{Operation: "compress", Path: "config", Paths: []string{"config/a.txt", "config/b.txt"}, Destination: "config/selected.tar.gz"})
	if err != nil {
		t.Fatal(err)
	}

	zipped, err := gzip.NewReader(strings.NewReader(mockFileContent(record.Files["config/selected.tar.gz"])))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = zipped.Close() }()

	archive := tar.NewReader(zipped)
	for _, name := range []string{"a.txt", "b.txt"} {
		header, err := archive.Next()
		if err != nil || header.Name != name {
			t.Fatal("unexpected selected archive entry", header, err)
		}

		body, err := io.ReadAll(archive)
		if err != nil || string(body) != record.Files["config/"+name] {
			t.Fatal("archive changed file contents", string(body), err)
		}
	}

	if _, err := archive.Next(); err != io.EOF {
		t.Fatal("archive included unselected files", err)
	}
}

func TestMockSelectedZIPProducesDownloadableArchive(t *testing.T) {
	record := runtimeRecord{Files: map[string]string{
		"config/": "", "config/a.txt": "first", "config/b.txt": "second", "config/ignored.txt": "skip",
	}}
	if err := mockCompress(t.Context(), &record, model.FileRequest{Operation: model.FileOperationCompress, Path: "config", Paths: []string{"config/a.txt", "config/b.txt"}, Destination: "config/selected.zip"}); err != nil {
		t.Fatal(err)
	}

	data := mockFileContent(record.Files["config/selected.zip"])

	archive, err := zip.NewReader(strings.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	if len(archive.File) != 2 {
		t.Fatal("included unselected files", archive.File)
	}

	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}

		body, err := io.ReadAll(reader)
		_ = reader.Close()

		if err != nil || string(body) != record.Files["config/"+file.Name] {
			t.Fatal("invalid ZIP contents", string(body), err)
		}
	}
}

func TestMockDeletesNonEmptyFolders(t *testing.T) {
	runtime, _, _, servers := fixture(t)
	server := servers[0]

	call := func(q model.FileRequest) (model.FileResult, error) {
		q.ServerID = server.ID
		return runtime.Files(t.Context(), server.SparkID, q)
	}
	for _, q := range []model.FileRequest{
		{Operation: "mkdir", Path: "folder"},
		{Operation: "mkdir", Path: "folder/nested"},
		{Operation: "write", Path: "folder/nested/save.txt", Revision: "absent", Data: []byte("saved")},
		{Operation: "write", Path: "folder-other.txt", Revision: "absent", Data: []byte("keep")},
		{Operation: "delete", Path: "folder"},
	} {
		if _, err := call(q); err != nil {
			t.Fatal(q, err)
		}
	}

	for _, q := range []model.FileRequest{
		{Operation: "list", Path: "folder"},
		{Operation: "read", Path: "folder/nested/save.txt"},
		{Operation: "delete", Path: "."},
	} {
		if _, err := call(q); err == nil {
			t.Fatal("deleted folder remained accessible or root deletion succeeded", q)
		}
	}

	if result, err := call(model.FileRequest{Operation: "read", Path: "folder-other.txt"}); err != nil || string(result.Data) != "keep" {
		t.Fatal("deletion affected a sibling file", result, err)
	}
}

func TestMockMovesFoldersWithContentsAndRejectsOverwrite(t *testing.T) {
	runtime, _, _, servers := fixture(t)
	server := servers[0]

	call := func(q model.FileRequest) (model.FileResult, error) {
		q.ServerID = server.ID
		return runtime.Files(t.Context(), server.SparkID, q)
	}
	for _, q := range []model.FileRequest{
		{Operation: "mkdir", Path: "source-folder"},
		{Operation: "mkdir", Path: "target-folder"},
		{Operation: "write", Path: "source-folder/content.txt", Revision: "absent", Data: []byte("saved")},
		{Operation: "rename", Path: "source-folder", Destination: "target-folder/source-folder"},
	} {
		if _, err := call(q); err != nil {
			t.Fatal(q, err)
		}
	}

	read, err := call(model.FileRequest{Operation: "read", Path: "target-folder/source-folder/content.txt"})
	if err != nil || string(read.Data) != "saved" {
		t.Fatal("folder move lost contents", err)
	}

	if _, err := call(model.FileRequest{Operation: "rename", Path: "target-folder/source-folder/content.txt", Destination: "target-folder/source-folder"}); err == nil {
		t.Fatal("folder move overwrote existing destination")
	}
}
