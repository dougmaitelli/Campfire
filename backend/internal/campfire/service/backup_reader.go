package service

import (
	"context"
	"errors"
	"io"

	"campfire/internal/shared/model"
)

// backupChunkReader adapts bounded archive reads to streaming inspection and S3
// synchronization. Callers retain their own authorization and workflow policy.
type backupChunkReader struct {
	ctx          context.Context
	size, offset int64
	data         []byte
	read         func(context.Context, int64) (model.FileResult, error)
}

func (r *backupChunkReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	if len(r.data) == 0 {
		if r.offset == r.size {
			return 0, io.EOF
		}

		chunk, err := r.read(r.ctx, r.offset)
		if err != nil {
			return 0, err
		}

		if chunk.Size != r.size || len(chunk.Data) == 0 || int64(len(chunk.Data)) > r.size-r.offset {
			return 0, errors.New("backup changed while reading")
		}

		r.data = chunk.Data
	}

	n := copy(p, r.data)
	r.data = r.data[n:]
	r.offset += int64(n)

	return n, nil
}
