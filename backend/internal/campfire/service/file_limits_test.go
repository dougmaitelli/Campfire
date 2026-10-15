package service

import (
	"context"
	"errors"
	"testing"

	"campfire/internal/shared/model"
)

func TestUploadLimitCannotBeOverriddenByRequests(t *testing.T) {
	deployment := &DeploymentService{MaxUploadBytes: 2 << 20}
	for _, request := range []model.FileRequest{
		{Operation: "upload.begin", Size: 3 << 20, UploadLimit: model.FileTransferLimit},
		{Operation: "upload.chunk", Offset: 2 << 20, Data: []byte("x"), UploadLimit: model.FileTransferLimit},
	} {
		_, err := deployment.Files(context.Background(), "game", request)

		var failure *model.FileError
		if !errors.As(err, &failure) || failure.Code != "file_limit" {
			t.Fatalf("upload limit bypass: %v", err)
		}
	}

	if (&DeploymentService{}).UploadLimit() != model.DefaultUploadLimit {
		t.Fatal("incorrect default upload limit")
	}
}
