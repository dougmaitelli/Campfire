package runtime

import (
	"encoding/binary"
	"io"
	"os"
)

// checkZIPDirectory bounds entry count before archive/zip allocates metadata.
// Scan the directory too: ZIP's declared count alone is not trustworthy.
func checkZIPDirectory(file *os.File, size int64) error {
	failure := fileFailure("invalid_file", "ZIP directory is invalid or exceeds 100,000 entries or 128 MiB of metadata.")
	if size < 22 {
		return failure
	}

	end := make([]byte, int(min(size, 65557)))
	if _, err := file.ReadAt(end, size-int64(len(end))); err != nil {
		return failure
	}

	index := -1

	for i := len(end) - 22; i >= 0; i-- {
		if binary.LittleEndian.Uint32(end[i:]) == 0x06054b50 && i+22+int(binary.LittleEndian.Uint16(end[i+20:])) == len(end) {
			index = i
			break
		}
	}

	if index < 0 {
		return failure
	}

	eocd := end[index:]
	if binary.LittleEndian.Uint16(eocd[4:]) != 0 || binary.LittleEndian.Uint16(eocd[6:]) != 0 {
		return failure
	}

	count := uint64(binary.LittleEndian.Uint16(eocd[10:]))
	length := uint64(binary.LittleEndian.Uint32(eocd[12:]))
	offset := uint64(binary.LittleEndian.Uint32(eocd[16:]))
	endPosition := uint64(size - int64(len(end)) + int64(index))

	if count == 65535 || length == 0xffffffff || offset == 0xffffffff {
		var locator [20]byte

		position := size - int64(len(end)) + int64(index) - 20
		if position < 0 {
			return failure
		}

		if _, err := file.ReadAt(locator[:], position); err != nil {
			return failure
		}

		if binary.LittleEndian.Uint32(locator[:]) != 0x07064b50 || binary.LittleEndian.Uint32(locator[4:]) != 0 || binary.LittleEndian.Uint32(locator[16:]) != 1 {
			return failure
		}

		address := binary.LittleEndian.Uint64(locator[8:])

		endPosition = address
		if address > uint64(size-56) {
			return failure
		}

		var record [56]byte
		if _, err := file.ReadAt(record[:], int64(address)); err != nil {
			return failure
		}

		if binary.LittleEndian.Uint32(record[:]) != 0x06064b50 || binary.LittleEndian.Uint32(record[16:]) != 0 || binary.LittleEndian.Uint32(record[20:]) != 0 {
			return failure
		}

		count, length, offset = binary.LittleEndian.Uint64(record[32:]), binary.LittleEndian.Uint64(record[40:]), binary.LittleEndian.Uint64(record[48:])
	}

	if count > backupEntries || length > 128<<20 || offset > uint64(size) || length > uint64(size)-offset || offset+length != endPosition {
		return failure
	}

	directory := io.NewSectionReader(file, int64(offset), int64(length))

	var actual uint64

	remaining := int64(length)
	for remaining > 0 {
		var header [46]byte
		if remaining < int64(len(header)) {
			return failure
		}

		if _, err := io.ReadFull(directory, header[:]); err != nil {
			return failure
		}

		if binary.LittleEndian.Uint32(header[:]) != 0x02014b50 {
			return failure
		}

		actual++
		if actual > backupEntries {
			return failure
		}

		extra := int64(binary.LittleEndian.Uint16(header[28:])) + int64(binary.LittleEndian.Uint16(header[30:])) + int64(binary.LittleEndian.Uint16(header[32:]))

		remaining -= int64(len(header))
		if extra > remaining {
			return failure
		}

		if _, err := directory.Seek(extra, io.SeekCurrent); err != nil {
			return failure
		}

		remaining -= extra
	}

	if actual != count {
		return failure
	}

	return nil
}
