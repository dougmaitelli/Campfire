package backuparchive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"testing"

	"campfire/internal/shared/model"
)

func portableFixture() Manifest {
	ember := model.Ember{ID: "test-ember", Schema: 1, Name: "Test game", Images: map[string]string{"default": "alpine:latest"}, Startup: "./server", WorkDir: "/home/container", User: "1000:1000", Variables: []model.Variable{{Name: "TOKEN", Default: "default", Hidden: true, Editable: true}}}
	server := model.Server{ServerConfiguration: model.ServerConfiguration{Name: "Test server", SparkID: "source-spark", EmberID: ember.ID, Image: "alpine:latest", Variables: map[string]string{"TOKEN": "private-token"}, CPULimit: 1, MemoryMB: 1024, Ports: []model.ServerPort{{BindIP: "0.0.0.0", HostPort: 1234, ContainerPort: 1234, Protocol: model.NetworkUDP}}}}

	return Manifest{Format: Format, BackupOptions: model.BackupOptions{Mode: model.BackupModeFull, Definition: model.NewBackupDefinition(server, ember)}}
}

func archiveFixture(t *testing.T, manifest Manifest, headers ...*tar.Header) []byte {
	t.Helper()

	var data bytes.Buffer

	compressed := gzip.NewWriter(&data)
	archive := tar.NewWriter(compressed)

	metadata, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	if err := archive.WriteHeader(&tar.Header{Name: model.BackupMetadataName, Typeflag: tar.TypeReg, Size: int64(len(metadata)), Mode: 0600}); err != nil {
		t.Fatal(err)
	}

	if _, err := archive.Write(metadata); err != nil {
		t.Fatal(err)
	}

	for _, header := range headers {
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

	return data.Bytes()
}

func TestPortableDefinitionRetainsSecretsAndExcludesPlacement(t *testing.T) {
	manifest := portableFixture()
	data := archiveFixture(t, manifest, &tar.Header{Name: "world.dat", Typeflag: tar.TypeReg, Mode: 0444})

	read, err := Inspect(t.Context(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	configuration := read.Definition.Configuration
	if configuration.Variables["TOKEN"] != "private-token" || configuration.SparkID != "" || len(configuration.Ports) != 0 {
		t.Fatal("portable configuration lost credentials or retained placement")
	}
}

func TestPortableImportRejectsUnsafeOrInconsistentArchives(t *testing.T) {
	for _, name := range []string{"../outside", "/absolute", "world/../../outside", model.BackupMetadataName} {
		t.Run(name, func(t *testing.T) {
			if _, err := Inspect(t.Context(), bytes.NewReader(archiveFixture(t, portableFixture(), &tar.Header{Name: name, Typeflag: tar.TypeReg}))); err == nil {
				t.Fatal("accepted unsafe entry")
			}
		})
	}

	manifest := portableFixture()
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeFifo} {
		if _, err := Inspect(t.Context(), bytes.NewReader(archiveFixture(t, manifest, &tar.Header{Name: "link", Typeflag: kind, Linkname: "outside"}))); err == nil {
			t.Fatal("accepted link or special entry")
		}
	}

	data := archiveFixture(t, manifest, &tar.Header{Name: "world", Typeflag: tar.TypeReg}, &tar.Header{Name: "world", Typeflag: tar.TypeReg})
	if _, err := Inspect(t.Context(), bytes.NewReader(data)); err == nil {
		t.Fatal("accepted duplicate entry")
	}

	data = archiveFixture(t, manifest)

	data[len(data)-5] ^= 0xff
	if _, err := Inspect(t.Context(), bytes.NewReader(data)); err == nil {
		t.Fatal("accepted corrupt checksum")
	}

	manifest.Format = 1
	if _, err := Inspect(t.Context(), bytes.NewReader(archiveFixture(t, manifest))); err == nil {
		t.Fatal("accepted nonportable format")
	}

	manifest = portableFixture()

	manifest.Mode, manifest.Paths = model.BackupModeSavedata, []model.SavedataPath{{Path: "world", Required: true}}
	if _, err := Inspect(t.Context(), bytes.NewReader(archiveFixture(t, manifest, &tar.Header{Name: "server.bin", Typeflag: tar.TypeReg}))); err == nil {
		t.Fatal("accepted data outside selected paths")
	}

	if _, err := Inspect(t.Context(), bytes.NewReader(archiveFixture(t, manifest))); err == nil {
		t.Fatal("accepted missing required save data")
	}
}
