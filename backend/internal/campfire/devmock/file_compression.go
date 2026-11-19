//go:build !production

package devmock

import (
	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
	"context"
	"slices"
	"time"
)

type mockCompression struct {
	request model.FileRequest
	expires time.Time
}

// Tiny SQLite-backed mock archives complete synchronously. Retain owner-bound
// acknowledgements so retries/status preserve the real publication contract.
func (r *Runtime) compression(ctx context.Context, record *runtimeRecord, q model.FileRequest) (model.FileResult, error) {
	result := model.FileResult{ServerID: q.ServerID, Path: q.Destination}
	if q.Compression == nil || len(q.Compression.ID) < 32 || !utils.IsValidResourceID(q.Compression.ID) || q.CompressionOwner == "" {
		return result, fileError(model.FileInvalid, "Invalid compression operation.")
	}

	if r.compressions == nil {
		r.compressions = make(map[string]mockCompression)
	}

	for id, item := range r.compressions {
		if !time.Now().Before(item.expires) {
			delete(r.compressions, id)
		}
	}

	if item, ok := r.compressions[q.Compression.ID]; ok {
		if item.request.ServerID != q.ServerID || item.request.CompressionOwner != q.CompressionOwner || item.request.Path != q.Path || item.request.Destination != q.Destination || !slices.Equal(item.request.Paths, q.Paths) {
			return result, fileError(model.FileNotFound, "Compression operation not found.")
		}
	} else {
		if q.Operation != model.FileOperationCompressStart {
			return result, fileError(model.FileNotFound, "Compression result is unavailable.")
		}

		if len(r.compressions) >= 128 {
			return result, fileError(model.FileBusy, "Too many recent compression results.")
		}

		if err := mockCompress(ctx, record, q); err != nil {
			return result, err
		}

		if err := r.db.WithContext(ctx).Save(record).Error; err != nil {
			return result, err
		}

		request := model.FileRequest{ServerID: q.ServerID, Path: q.Path, Paths: slices.Clone(q.Paths), Destination: q.Destination, CompressionOwner: q.CompressionOwner}
		r.compressions[q.Compression.ID] = mockCompression{request: request, expires: time.Now().Add(limits.ArchiveResultRetention)}
	}

	result.Compression = &model.CompressionStatus{ID: q.Compression.ID, State: model.CompressionCompleted}

	return result, nil
}
