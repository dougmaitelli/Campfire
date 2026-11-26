//go:build !production

package devmock

import (
	"errors"
	"testing"
	"time"

	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
)

func TestMockCompressionUsesSharedResultRetention(t *testing.T) {
	runtime, _, _, servers := fixture(t)
	server := onlineServer(t, runtime, servers)
	call := func(q model.FileRequest) (model.FileResult, error) {
		q.ServerID = server.ID
		return runtime.Files(t.Context(), server.SparkID, q)
	}

	if _, err := call(model.FileRequest{Operation: model.FileOperationWrite, Path: "retention-source.txt", Revision: "absent", Data: []byte("payload")}); err != nil {
		t.Fatal(err)
	}

	q := model.FileRequest{Operation: model.FileOperationCompressStart, Path: "retention-source.txt", Destination: "retention.tar.gz", CompressionOwner: "owner", Compression: &model.CompressionRequest{ID: "12345678901234567890123456789012"}}
	before := time.Now()

	result, err := call(q)
	if err != nil || result.Compression.State != model.CompressionCompleted {
		t.Fatal(result, err)
	}

	after := time.Now()

	runtime.mu.Lock()
	expires := runtime.compressions[q.Compression.ID].expires
	runtime.mu.Unlock()

	if expires.Before(before.Add(limits.ArchiveResultRetention)) || expires.After(after.Add(limits.ArchiveResultRetention)) {
		t.Fatal("mock does not use the shared retention policy", expires)
	}

	q.Operation = model.FileOperationCompressStatus
	if _, err := call(q); err != nil {
		t.Fatal(err)
	}

	runtime.mu.Lock()
	retained := runtime.compressions[q.Compression.ID]
	unchanged := retained.expires.Equal(expires)
	retained.expires = time.Now().Add(-time.Second)
	runtime.compressions[q.Compression.ID] = retained
	runtime.mu.Unlock()

	if !unchanged {
		t.Fatal("polling renewed result retention")
	}

	var failure *model.FileError
	if _, err := call(q); !errors.As(err, &failure) || failure.Code != model.FileNotFound {
		t.Fatal("expired result remained available", err)
	}

	if _, err := call(model.FileRequest{Operation: model.FileOperationStat, Path: q.Destination}); err != nil {
		t.Fatal("expiring the result removed the archive", err)
	}
}
