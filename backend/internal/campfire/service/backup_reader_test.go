package service

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"

	"campfire/internal/shared/model"
)

func TestBackupChunkReaderBuffersAndStopsAtDeclaredSize(t *testing.T) {
	data := []byte("abcdefgh")

	var offsets []int64

	reader := &backupChunkReader{ctx: t.Context(), size: int64(len(data)), read: func(_ context.Context, offset int64) (model.FileResult, error) {
		offsets = append(offsets, offset)
		return model.FileResult{Size: int64(len(data)), Data: data[offset:min(offset+3, int64(len(data)))]}, nil
	}}
	if n, err := reader.Read(nil); n != 0 || err != nil || len(offsets) != 0 {
		t.Fatal("zero read performed I/O", n, err, offsets)
	}

	var got []byte

	buffer := make([]byte, 2)
	for {
		n, err := reader.Read(buffer)
		got = append(got, buffer[:n]...)

		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatal(err)
		}
	}

	if string(got) != string(data) || !slices.Equal(offsets, []int64{0, 3, 6}) {
		t.Fatal(string(got), offsets)
	}

	if n, err := reader.Read(buffer); n != 0 || !errors.Is(err, io.EOF) || len(offsets) != 3 {
		t.Fatal("read beyond archive", n, err, offsets)
	}
}

func TestBackupChunkReaderRejectsChangedOrIncompleteChunks(t *testing.T) {
	for name, chunk := range map[string]model.FileResult{
		"size changed": {Size: 5, Data: []byte("abcd")},
		"empty chunk":  {Size: 4},
		"excess bytes": {Size: 4, Data: []byte("abcde")},
	} {
		t.Run(name, func(t *testing.T) {
			reader := &backupChunkReader{ctx: t.Context(), size: 4, read: func(context.Context, int64) (model.FileResult, error) { return chunk, nil }}
			if n, err := reader.Read(make([]byte, 10)); n != 0 || err == nil || reader.offset != 0 {
				t.Fatal(n, err, reader.offset)
			}
		})
	}
}

func TestBackupChunkReaderPropagatesSourceCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	reader := &backupChunkReader{ctx: ctx, size: 4, read: func(ctx context.Context, _ int64) (model.FileResult, error) { return model.FileResult{}, ctx.Err() }}
	if n, err := reader.Read(make([]byte, 4)); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal(n, err)
	}
}
