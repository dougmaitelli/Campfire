//go:build production

package sparkbundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/elf"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"campfire/internal/shared/buildinfo"
	"campfire/internal/shared/protocol"
)

func TestBundledArchitecturesMatchPanel(t *testing.T) {
	if !Available() {
		t.Fatal("production panel must bundle both architectures")
	}

	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			name := "spark_linux_" + arch + ".tar.gz"

			data, err := fs.ReadFile(assets(), name)
			if err != nil {
				t.Fatal(err)
			}

			checksum, err := fs.ReadFile(assets(), name+".sha256")
			if err != nil || string(checksum) != fmt.Sprintf("%x  %s\n", sha256.Sum256(data), name) {
				t.Fatal("bundle checksum mismatch", err)
			}

			zipped, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = zipped.Close() }()

			reader := tar.NewReader(zipped)
			foundBinary, foundUnit := false, false

			for {
				header, err := reader.Next()
				if errors.Is(err, io.EOF) {
					break
				}

				if err != nil {
					t.Fatal(err)
				}

				if header.Name == "./deploy/systemd/spark.service" {
					foundUnit = true
				}

				if header.Name != "./spark" {
					continue
				}

				foundBinary = true

				binary, err := io.ReadAll(reader)
				if err != nil {
					t.Fatal(err)
				}

				image, err := elf.NewFile(bytes.NewReader(binary))
				if err != nil {
					t.Fatal(err)
				}

				machine := map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64}[arch]
				if image.Machine != machine {
					t.Fatal("wrong executable architecture", image.Machine)
				}

				if runtime.GOOS != "linux" || runtime.GOARCH != arch {
					continue
				}

				filename := filepath.Join(t.TempDir(), "spark")
				if err := os.WriteFile(filename, binary, 0755); err != nil {
					t.Fatal(err)
				}

				output, err := exec.CommandContext(t.Context(), filename, "--update-info").Output()

				var info protocol.SparkUpdateInfo
				if err != nil || json.Unmarshal(output, &info) != nil || info.Version != buildinfo.SparkVersion() || info.Protocol != protocol.ProtocolVersion {
					t.Fatal("bundled Spark does not match the panel", string(output), err)
				}
			}

			if !foundBinary || !foundUnit {
				t.Fatal("incomplete Spark package")
			}
		})
	}
}
