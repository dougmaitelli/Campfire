package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"testing"

	"campfire/internal/shared/model"
)

func TestTransferArchiveRejectsUnsafeEntries(t *testing.T) {
	for _, test := range []struct {
		name          string
		header        tar.Header
		duplicate     bool
		wrongIdentity bool
	}{
		{name: "traversal", header: tar.Header{Name: "data/../../escape", Typeflag: tar.TypeReg}},
		{name: "absolute", header: tar.Header{Name: "/data/escape", Typeflag: tar.TypeReg}},
		{name: "symlink", header: tar.Header{Name: "data/link", Typeflag: tar.TypeSymlink, Linkname: "../../escape"}},
		{name: "hardlink", header: tar.Header{Name: "data/link", Typeflag: tar.TypeLink, Linkname: "data/file"}},
		{name: "device", header: tar.Header{Name: "data/device", Typeflag: tar.TypeChar}},
		{name: "outside namespace", header: tar.Header{Name: "state.json", Typeflag: tar.TypeReg}},
		{name: "duplicate", header: tar.Header{Name: "data/file", Typeflag: tar.TypeReg}, duplicate: true},
		{name: "identity", header: tar.Header{Name: "data/file", Typeflag: tar.TypeReg}, wrongIdentity: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var buffer bytes.Buffer

			compressed := gzip.NewWriter(&buffer)
			archive := tar.NewWriter(compressed)

			manifest := transferManifest{Version: 1, ID: "transfer", ServerID: "server", EmberID: "ember"}
			if test.wrongIdentity {
				manifest.ServerID = "other"
			}

			body, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}

			if err := archive.WriteHeader(&tar.Header{Name: "manifest.json", Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
				t.Fatal(err)
			}

			if _, err := archive.Write(body); err != nil {
				t.Fatal(err)
			}

			if err := archive.WriteHeader(&test.header); err != nil {
				t.Fatal(err)
			}

			if test.duplicate {
				if err := archive.WriteHeader(&test.header); err != nil {
					t.Fatal(err)
				}
			}

			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}

			if err := compressed.Close(); err != nil {
				t.Fatal(err)
			}

			deployment := model.Deployment{ServerID: "server"}
			deployment.Configuration.EmberID = "ember"

			deployment.Configuration.DiskMB = 1
			if _, err := extractTransfer(context.Background(), &buffer, t.TempDir(), deployment, "transfer"); err == nil {
				t.Fatal("unsafe transfer accepted")
			}
		})
	}
}
