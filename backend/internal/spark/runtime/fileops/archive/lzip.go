package archive

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"

	"github.com/ulikunitz/xz/lzma"
)

// lzipReader streams v1 members, validating their CRC, decoded length and member
// length. Only the small framing differs from LZMA; no compressed member is buffered.
type lzipReader struct {
	input  *bufio.Reader
	member *lzma.Reader
	count  *countedReader
	crc    uint32
	size   uint64
	err    error
}

type countedReader struct {
	io.Reader
	n uint64
}

func (r *countedReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.n += uint64(n)

	return n, err
}

func newLzipReader(input io.Reader) (*lzipReader, error) {
	r := &lzipReader{input: bufio.NewReader(input)}
	if err := r.next(); err != nil {
		return nil, incompleteLzip(err)
	}

	return r, nil
}

func (r *lzipReader) next() error {
	var header [6]byte
	if _, err := io.ReadFull(r.input, header[:]); err != nil {
		return err
	}

	if string(header[:4]) != "LZIP" {
		return errors.New("invalid lzip header")
	}

	if header[4] != 1 {
		return errDecoderOptions
	}

	dict := uint32(1) << (header[5] & 31)

	dict -= (dict / 16) * uint32(header[5]>>5)
	if dict > decoderMemoryLimit {
		return errDecoderMemory
	}

	if dict < 4096 {
		return errors.New("invalid lzip dictionary")
	}

	var classic [lzma.HeaderLen]byte

	classic[0] = 0x5d // lzip fixes lc=3, lp=0, pb=2.
	binary.LittleEndian.PutUint32(classic[1:5], dict)

	for i := 5; i < len(classic); i++ {
		classic[i] = 0xff
	} // EOS marker, unknown size.

	r.count = &countedReader{Reader: r.input, n: 6}

	member, err := (lzma.ReaderConfig{DictCap: decoderMemoryLimit}).NewReader(io.MultiReader(bytes.NewReader(classic[:]), r.count))
	if err != nil {
		return incompleteLzip(err)
	}

	r.member, r.crc, r.size = member, 0, 0

	return nil
}

func incompleteLzip(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}

	return err
}

func (r *lzipReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	for {
		if r.err != nil {
			return 0, r.err
		}

		n, err := r.member.Read(p)
		r.crc = crc32.Update(r.crc, crc32.IEEETable, p[:n])
		r.size += uint64(n)

		if errors.Is(err, io.EOF) {
			var trailer [20]byte

			_, err = io.ReadFull(r.count, trailer[:])

			err = incompleteLzip(err)
			if err == nil && (binary.LittleEndian.Uint32(trailer[:4]) != r.crc || binary.LittleEndian.Uint64(trailer[4:12]) != r.size || binary.LittleEndian.Uint64(trailer[12:]) != r.count.n) {
				err = errors.New("invalid lzip checksum or length")
			}

			if err == nil {
				err = r.next()
			}
		}

		r.err = err
		if n > 0 || err != nil {
			return n, err
		}
	}
}
