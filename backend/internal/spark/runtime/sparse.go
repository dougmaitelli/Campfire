package runtime

import (
	"bytes"
	"io"
	"os"
)

// Portable archives contain the logical byte stream, including holes. Reuse
// one buffer across entries and omit zero blocks when restoring a new file.
type sparseFileCopier [64 << 10]byte

var zeroBlock [4096]byte

func (buf *sparseFileCopier) copy(file *os.File, src io.Reader, size int64) error {
	for offset := int64(0); offset < size; {
		chunk := buf[:min(int64(len(buf)), size-offset)]
		if _, err := io.ReadFull(src, chunk); err != nil {
			return err
		}

		start := 0

		for pos := 0; pos < len(chunk); pos += len(zeroBlock) {
			end := min(pos+len(zeroBlock), len(chunk))
			if !bytes.Equal(chunk[pos:end], zeroBlock[:end-pos]) {
				continue
			}

			if start < pos {
				if _, err := file.WriteAt(chunk[start:pos], offset+int64(start)); err != nil {
					return err
				}
			}

			start = end
		}

		if start < len(chunk) {
			if _, err := file.WriteAt(chunk[start:], offset+int64(start)); err != nil {
				return err
			}
		}

		offset += int64(len(chunk))
	}

	return file.Truncate(size)
}
