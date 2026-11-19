package service

import (
	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
	"context"
	"errors"
	"testing"
)

func TestTransferFenceAllowsCompressionStatusAndCancellation(t *testing.T) {
	f := newTransferFixture(t)
	calls := 0

	f.service.Deployment.Remote = &extractionTransferRouter{transferRouter: f.router, fileCall: func(_ context.Context, _ string, q model.FileRequest) (model.FileResult, error) {
		calls++
		return model.FileResult{ServerID: q.ServerID, Path: q.Path, Compression: &model.CompressionStatus{ID: q.Compression.ID, State: "running"}}, nil
	}}
	if _, err := f.service.Begin(t.Context(), f.server.ID, "target", f.ports); err != nil {
		t.Fatal(err)
	}

	q := model.FileRequest{Path: "source", Destination: "archive.tar.gz", CompressionOwner: "owner", Compression: &model.CompressionRequest{ID: "12345678901234567890123456789012"}}
	for _, operation := range []model.FileOperation{model.FileOperationCompressStatus, model.FileOperationCompressCancel} {
		q.Operation = operation
		if _, err := f.service.Deployment.Files(t.Context(), f.server.ID, q); err != nil {
			t.Fatal("control request fenced", operation, err)
		}
	}

	q.Operation = model.FileOperationCompressStart
	if _, err := f.service.Deployment.Files(t.Context(), f.server.ID, q); !errors.Is(err, repository.ErrJobActive) {
		t.Fatal("new compression bypassed transfer fence", err)
	}

	if calls != 2 {
		t.Fatal("unexpected dispatch", calls)
	}
}
