package fileops

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"campfire/internal/shared/model"
)

func TestZIPUnsupportedVariantsHaveActionableErrors(t *testing.T) {
	var buffer bytes.Buffer

	writer := zip.NewWriter(&buffer)
	if _, err := writer.Create("empty.txt"); err != nil {
		t.Fatal(err)
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	central := bytes.Index(buffer.Bytes(), []byte{0x50, 0x4b, 0x01, 0x02})
	if central < 0 {
		t.Fatal("missing fixture directory")
	}

	for _, item := range []struct {
		name    string
		flags   uint16
		method  uint16
		message string
	}{
		{"encrypted", 1, zip.Deflate, "Password-protected"},
		{"strong-encryption", 0x40, zip.Deflate, "Password-protected"},
		{"masked-headers", 0x2000, zip.Deflate, "Password-protected"},
		{"unsupported-method", 0, 99, "Unsupported ZIP compression method"},
	} {
		t.Run(item.name, func(t *testing.T) {
			data := bytes.Clone(buffer.Bytes())
			binary.LittleEndian.PutUint16(data[6:], item.flags)
			binary.LittleEndian.PutUint16(data[central+8:], item.flags)
			binary.LittleEndian.PutUint16(data[8:], item.method)
			binary.LittleEndian.PutUint16(data[central+10:], item.method)
			err := readArchiveFixture(t.Context(), t, "zip", data)

			var failure *model.FileError
			if !errors.As(err, &failure) || failure.Code != model.FileInvalid || !strings.Contains(err.Error(), item.message) {
				t.Fatal(err)
			}
		})
	}
}
