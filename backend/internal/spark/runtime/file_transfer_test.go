package runtime

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"campfire/internal/shared/model"
)

func TestChunkedFilesAtomicPublicationAndConflict(t *testing.T) {
	m, d := fileTestManager(t)
	call := func(q model.FileRequest) (model.FileResult, error) {
		q.ServerID = d.ServerID
		return m.Files(context.Background(), d, q)
	}
	q := model.FileRequest{Operation: "upload.begin", Path: "world.bin", Revision: "absent", Size: 2*model.FileLimit + 7}

	session, err := call(q)
	if err != nil {
		t.Fatal(err)
	}

	q.UploadID, q.Operation = session.UploadID, "upload.chunk"

	q.Offset, q.Data = 2*model.FileLimit, []byte("the end")
	if _, err := call(q); err != nil {
		t.Fatal(err)
	}

	q.Operation = "upload.commit"
	if _, err := call(q); err == nil {
		t.Fatal("incomplete sparse upload was published")
	}

	if _, err := os.Stat(filepath.Join(m.root(d.ServerID), "data/world.bin")); !os.IsNotExist(err) {
		t.Fatal("uncommitted destination exists")
	}

	q.Operation, q.Offset, q.Data = "upload.chunk", 0, bytes.Repeat([]byte{17}, model.FileLimit)
	if _, err := call(q); err != nil {
		t.Fatal(err)
	}

	q.Offset = model.FileLimit
	if _, err := call(q); err != nil {
		t.Fatal(err)
	}

	q.Operation = "upload.commit"
	if _, err := call(q); err != nil {
		t.Fatal(err)
	}

	info, err := call(model.FileRequest{Operation: "stat", Path: "world.bin"})
	if err != nil || info.Size != 2*model.FileLimit+7 {
		t.Fatalf("stat: %+v %v", info, err)
	}

	for offset := int64(0); offset < info.Size; {
		part, err := call(model.FileRequest{Operation: "read.chunk", Path: "world.bin", Revision: info.Revision, Offset: offset})
		if err != nil || len(part.Data) == 0 || len(part.Data) > model.FileLimit {
			t.Fatalf("chunk: %v", err)
		}

		offset += int64(len(part.Data))
	}

	session, err = call(model.FileRequest{Operation: "upload.begin", Path: "world.bin", Revision: info.Revision, Size: 0})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(m.root(d.ServerID), "data/world.bin"), []byte("external edit"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := call(model.FileRequest{Operation: "upload.commit", Path: "world.bin", UploadID: session.UploadID}); err == nil {
		t.Fatal("overwrote changed destination")
	}

	if _, err := call(model.FileRequest{Operation: "upload.cancel", Path: "world.bin", UploadID: session.UploadID}); err != nil {
		t.Fatal(err)
	}

	if _, err := call(model.FileRequest{Operation: "read.chunk", Path: "world.bin", Revision: info.Revision}); err == nil {
		t.Fatal("stale download accepted")
	}
}

func TestUnlimitedFilesAndArchivesWithoutQuotaSupport(t *testing.T) {
	m, d := fileTestManager(t)
	m.Quota = nil
	d.Configuration.DiskMB = 0
	call := func(q model.FileRequest) model.FileResult {
		t.Helper()

		q.ServerID = d.ServerID

		result, err := m.Files(t.Context(), d, q)
		if err != nil {
			t.Fatal(q.Operation, err)
		}

		return result
	}

	call(model.FileRequest{Operation: "write", Path: "world", Revision: "absent", Data: []byte("saved world")})
	call(model.FileRequest{Operation: "compress", Path: "world", Destination: "world.tar.gz"})
	call(model.FileRequest{Operation: "extract", Path: "world.tar.gz", Destination: "restored"})

	read := call(model.FileRequest{Operation: "read", Path: "restored/world"})
	if string(read.Data) != "saved world" {
		t.Fatal("unlimited archive did not preserve data")
	}

	upload := call(model.FileRequest{Operation: "upload.begin", Path: "upload", Revision: "absent", Size: 3})
	call(model.FileRequest{Operation: "upload.chunk", Path: "upload", UploadID: upload.UploadID, Data: []byte("abc")})
	call(model.FileRequest{Operation: "upload.commit", Path: "upload", UploadID: upload.UploadID})

	read = call(model.FileRequest{Operation: "read", Path: "upload"})
	if string(read.Data) != "abc" {
		t.Fatal("unlimited upload did not preserve data")
	}
}

func TestConfiguredUploadLimitForKnownAndUnknownSizes(t *testing.T) {
	m, d := fileTestManager(t)

	call := func(q model.FileRequest) (model.FileResult, error) {
		q.ServerID = d.ServerID
		return m.Files(context.Background(), d, q)
	}
	if _, err := call(model.FileRequest{Operation: "upload.begin", Path: "large", Revision: "absent", Size: model.DefaultUploadLimit + 1}); err == nil {
		t.Fatal("default upload ceiling not applied")
	}

	if _, err := call(model.FileRequest{Operation: "upload.begin", Path: "large", Revision: "absent", Size: 3 << 20, UploadLimit: 2 << 20}); err == nil {
		t.Fatal("declared upload exceeded configured limit")
	}

	for _, size := range []int64{-1, 2 << 20} {
		started, err := call(model.FileRequest{Operation: "upload.begin", Path: "upload", Revision: "absent", Size: size, UploadLimit: 2 << 20})
		if err != nil {
			t.Fatal(err)
		}

		if _, err := call(model.FileRequest{Operation: "upload.chunk", Path: "upload", UploadID: started.UploadID, Offset: 2 << 20, Data: []byte("x")}); err == nil {
			t.Fatal("unknown/deferred-size upload bypassed its ceiling")
		}

		if _, err := call(model.FileRequest{Operation: "upload.chunk", Path: "upload", UploadID: started.UploadID, Data: make([]byte, model.FileLimit)}); err != nil {
			t.Fatal(err)
		}

		if _, err := call(model.FileRequest{Operation: "upload.commit", Path: "upload", UploadID: started.UploadID, UploadLimit: 512 << 10}); err == nil {
			t.Fatal("upload ignored a lowered limit at commit")
		}

		if _, err := call(model.FileRequest{Operation: "upload.cancel", Path: "upload", UploadID: started.UploadID}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestArchivePreservesFileAndDirectoryPermissions(t *testing.T) {
	for _, format := range []string{"tar.gz", "zip"} {
		t.Run(format, func(t *testing.T) {
			m, d := fileTestManager(t)

			var data bytes.Buffer

			if format == "zip" {
				archive := zip.NewWriter(&data)

				for _, entry := range []struct {
					name string
					mode os.FileMode
				}{{"mods/", os.ModeDir | 0750}, {"mods/start.sh", 0755}} {
					header := &zip.FileHeader{Name: entry.name}
					header.SetMode(entry.mode)

					if _, err := archive.CreateHeader(header); err != nil {
						t.Fatal(err)
					}
				}

				if err := archive.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				compressed := gzip.NewWriter(&data)

				archive := tar.NewWriter(compressed)
				for _, header := range []*tar.Header{{Name: "mods/", Typeflag: tar.TypeDir, Mode: 0750}, {Name: "mods/start.sh", Typeflag: tar.TypeReg, Mode: 0755}} {
					if err := archive.WriteHeader(header); err != nil {
						t.Fatal(err)
					}
				}

				if err := archive.Close(); err != nil {
					t.Fatal(err)
				}

				if err := compressed.Close(); err != nil {
					t.Fatal(err)
				}
			}

			volume := filepath.Join(m.root(d.ServerID), "data")
			if err := os.WriteFile(filepath.Join(volume, "mods."+format), data.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}

			if _, err := m.Files(t.Context(), d, model.FileRequest{ServerID: d.ServerID, Operation: "extract", Path: "mods." + format, Destination: "unpacked"}); err != nil {
				t.Fatal(err)
			}

			for name, mode := range map[string]os.FileMode{"mods": 0750, "mods/start.sh": 0755} {
				info, err := os.Stat(filepath.Join(volume, "unpacked", name))
				if err != nil || info.Mode().Perm() != mode {
					t.Fatal("archive changed permissions", name, info, err)
				}
			}
		})
	}
}

func TestArchivesRoundTripAndContainment(t *testing.T) {
	m, d := fileTestManager(t)

	volume := filepath.Join(m.root(d.ServerID), "data")
	if err := os.MkdirAll(filepath.Join(volume, "config"), 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(volume, "config/settings"), bytes.Repeat([]byte("x"), 2*model.FileLimit), 0700); err != nil {
		t.Fatal(err)
	}

	call := func(operation, source, destination string) error {
		_, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: operation, Path: source, Destination: destination})
		return err
	}
	if err := call("compress", "config", "config.tar.gz"); err != nil {
		t.Fatal(err)
	}

	if err := call("extract", "config.tar.gz", "restored"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(volume, "restored/config/settings"))
	if err != nil || len(data) != 2*model.FileLimit {
		t.Fatalf("roundtrip: %v", err)
	}

	info, err := os.Stat(filepath.Join(volume, "restored/config/settings"))
	if err != nil || info.Mode().Perm()&0100 == 0 {
		t.Fatal("archive lost the executable bit", err)
	}

	for _, entry := range []string{"../escape", "/absolute", "a/../../escape", "a\\escape"} {
		var data bytes.Buffer

		zipped := gzip.NewWriter(&data)

		archive := tar.NewWriter(zipped)
		if err := archive.WriteHeader(&tar.Header{Name: entry, Typeflag: tar.TypeReg, Mode: 0600, Size: 1}); err != nil {
			t.Fatal(err)
		}

		if _, err := archive.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}

		if err := archive.Close(); err != nil {
			t.Fatal(err)
		}

		if err := zipped.Close(); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(volume, "bad.tar.gz"), data.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}

		if err := call("extract", "bad.tar.gz", "bad-output"); err == nil {
			t.Fatalf("accepted unsafe path %q", entry)
		}

		if _, err := os.Stat(filepath.Join(volume, "bad-output")); !os.IsNotExist(err) {
			t.Fatal("failed archive published")
		}
	}

	var zipped bytes.Buffer

	archive := zip.NewWriter(&zipped)

	writer, err := archive.Create("nested/config.json")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := writer.Write([]byte("{}")); err != nil {
		t.Fatal(err)
	}

	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(volume, "config.zip"), zipped.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	if err := call("extract", "config.zip", "unzipped"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(volume, "unzipped/nested/config.json")); err != nil {
		t.Fatal(err)
	}

	if err := call("extract", "config.zip", "unzipped"); err == nil {
		t.Fatal("replaced existing extraction directory")
	}

	// Reject misleading entry counts before handing the directory to archive/zip.
	bad := bytes.Clone(zipped.Bytes())
	binary.LittleEndian.PutUint16(bad[len(bad)-22+10:], 65534)

	if err := os.WriteFile(filepath.Join(volume, "bad.zip"), bad, 0600); err != nil {
		t.Fatal(err)
	}

	if err := call("extract", "bad.zip", "bad-zip-output"); err == nil {
		t.Fatal("accepted forged ZIP directory count")
	}
}

func TestArchiveRejectsLinksDuplicatesAndQuotaExpansion(t *testing.T) {
	for _, name := range []string{"symlink", "hardlink", "duplicate", "quota", "checksum"} {
		t.Run(name, func(t *testing.T) {
			m, d := fileTestManager(t)

			var data bytes.Buffer

			zipped := gzip.NewWriter(&data)
			archive := tar.NewWriter(zipped)
			header := &tar.Header{Name: "payload", Typeflag: tar.TypeReg, Mode: 0600}

			switch name {
			case "symlink":
				header.Typeflag, header.Linkname = tar.TypeSymlink, "../outside"
			case "hardlink":
				header.Typeflag, header.Linkname = tar.TypeLink, "../outside"
			case "quota":
				header.Size, d.Configuration.DiskMB = 2*model.FileLimit, 1
			}

			if err := archive.WriteHeader(header); err != nil {
				t.Fatal(err)
			}

			if header.Size > 0 {
				if _, err := archive.Write(make([]byte, int(header.Size))); err != nil {
					t.Fatal(err)
				}
			}

			if name == "duplicate" {
				if err := archive.WriteHeader(header); err != nil {
					t.Fatal(err)
				}
			}

			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}

			if err := zipped.Close(); err != nil {
				t.Fatal(err)
			}

			contents := data.Bytes()
			if name == "checksum" {
				contents[len(contents)-1] ^= 255
			}

			volume := filepath.Join(m.root(d.ServerID), "data")
			if err := os.WriteFile(filepath.Join(volume, "input.tar.gz"), contents, 0600); err != nil {
				t.Fatal(err)
			}

			_, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "extract", Path: "input.tar.gz", Destination: "output"})
			if err == nil {
				t.Fatal("unsafe archive accepted")
			}

			if _, err := os.Stat(filepath.Join(volume, "output")); !os.IsNotExist(err) {
				t.Fatal("failed archive published")
			}
		})
	}
}
