package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestOffHostRestoreVerifiesBytesAndPreservesOriginalOnFailure(t *testing.T) {
	valid := testArchive(t, []*tar.Header{{Name: "world", Typeflag: tar.TypeReg, Size: 4, Mode: 0600}})
	for _, test := range []struct {
		name      string
		body      []byte
		digest    string
		wantError bool
	}{
		{name: "valid", body: valid},
		{name: "bad checksum", body: valid, digest: strings.Repeat("0", 64), wantError: true},
		{name: "bad archive", body: []byte("not a gzip archive"), wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			m, d := fileTestManager(t)

			m.Ownership = func(string, string) error { return nil }
			if status := awaitOperation(t, m, d, "install", "install"); status.Error != "" {
				t.Fatal(status.Error)
			}

			world := filepath.Join(m.root(d.ServerID), "data", "world")
			if err := os.WriteFile(world, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(test.body) }))
			defer server.Close()

			hash := sha256.Sum256(test.body)

			digest := test.digest
			if digest == "" {
				digest = hex.EncodeToString(hash[:])
			}

			status := awaitBackup(t, m, d, model.RuntimeRequest{Action: "restore", OperationID: "remote-restore", Content: "offhost.tar.gz", BackupSource: &model.BackupSource{URL: server.URL + "?secret=private-signature", Size: int64(len(test.body)), Digest: digest}})
			if (status.Error != "") != test.wantError {
				t.Fatal(status.Error)
			}

			if strings.Contains(status.Error, "private-signature") || strings.Contains(status.Error, server.URL) {
				t.Fatal("signed URL leaked in status")
			}

			data, err := os.ReadFile(world)
			if err != nil {
				t.Fatal(err)
			}

			if test.wantError && !bytes.Equal(data, []byte("original")) {
				t.Fatal("failed restore changed original world")
			}

			if !test.wantError && !bytes.Equal(data, []byte("xxxx")) {
				t.Fatal("remote archive not restored")
			}

			if _, err := m.Files(context.Background(), d, model.FileRequest{ServerID: d.ServerID, Operation: "backup.delete", Path: "offhost.tar.gz"}); !test.wantError && err != nil {
				t.Fatal("individual backup deletion failed", err)
			}
		})
	}
}
