package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/campfire/database/repository"
	"campfire/internal/shared/model"
)

type extractionTransferRouter struct {
	*transferRouter
	fileCall func(context.Context, string, model.FileRequest) (model.FileResult, error)
}

func (r *extractionTransferRouter) Files(ctx context.Context, node string, q model.FileRequest) (model.FileResult, error) {
	return r.fileCall(ctx, node, q)
}

func TestTransferFenceAllowsExistingExtractionStatusAndCancel(t *testing.T) {
	f := newTransferFixture(t)

	ember, err := f.service.Deployment.Embers.Get(t.Context(), f.server.EmberID)
	if err != nil {
		t.Fatal(err)
	}

	d, err := model.NewDeployment(f.server.ID, f.server.ServerConfiguration, ember)
	if err != nil {
		t.Fatal(err)
	}

	var buffer bytes.Buffer

	archive := zip.NewWriter(&buffer)

	member, err := archive.Create("world.bin")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := member.Write([]byte("replacement")); err != nil {
		t.Fatal(err)
	}

	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	volume := filepath.Join(f.source.Directory, f.server.ID, "data")
	if err := os.WriteFile(filepath.Join(volume, "source.zip"), buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}

	q := model.FileRequest{ServerID: f.server.ID, Operation: model.FileOperationExtractStart, Path: "source.zip", Destination: ".", ExtractionOwner: "owner", Extraction: &model.ExtractionRequest{ID: "12345678901234567890123456789012"}}
	if _, err := f.source.Files(t.Context(), d, q); err != nil {
		t.Fatal(err)
	}

	q.Operation = model.FileOperationExtractStatus

	var waiting *model.ExtractionStatus

	for until := time.Now().Add(2 * time.Second); time.Now().Before(until); time.Sleep(time.Millisecond) {
		result, err := f.source.Files(t.Context(), d, q)
		if err != nil {
			t.Fatal(err)
		}

		if result.Extraction.State == "waiting" {
			waiting = result.Extraction
			break
		}
	}

	if waiting == nil {
		t.Fatal("extraction did not reach conflict wait")
	}

	f.service.Deployment.Remote = &extractionTransferRouter{transferRouter: f.router, fileCall: func(ctx context.Context, _ string, q model.FileRequest) (model.FileResult, error) {
		return f.source.Files(ctx, d, q)
	}}
	if _, err := f.service.Begin(t.Context(), f.server.ID, "target", f.ports); err != nil {
		t.Fatal(err)
	}

	if _, err := f.service.Deployment.Files(t.Context(), f.server.ID, q); err != nil {
		t.Fatal("transfer blocked extraction observation", err)
	}

	for _, operation := range []model.FileOperation{model.FileOperationExtractStart, model.FileOperationExtractResolve} {
		blocked := q
		blocked.Operation = operation

		blocked.Extraction = &model.ExtractionRequest{ID: waiting.ID, ConflictID: waiting.Conflict.ID, Action: "overwrite"}
		if _, err := f.service.Deployment.Files(t.Context(), f.server.ID, blocked); !errors.Is(err, repository.ErrJobActive) {
			t.Fatal("transfer did not fence extraction writes", operation, err)
		}
	}

	for _, operation := range []model.FileOperation{model.FileOperationExtractStatus, model.FileOperationExtractCancel} {
		wrongOwner := q
		wrongOwner.Operation, wrongOwner.ExtractionOwner = operation, "another-owner"

		var failure *model.FileError
		if _, err := f.service.Deployment.Files(t.Context(), f.server.ID, wrongOwner); !errors.As(err, &failure) || failure.Code != model.FileNotFound {
			t.Fatal("control request bypassed owner binding", operation, err)
		}
	}

	q.Operation = model.FileOperationExtractCancel

	result, err := f.service.Deployment.Files(t.Context(), f.server.ID, q)
	if err != nil || result.Extraction.State != "cancelled" {
		t.Fatal("transfer blocked extraction cancellation", result, err)
	}

	q.Operation = model.FileOperationExtractStatus

	result, err = f.service.Deployment.Files(t.Context(), f.server.ID, q)
	if err != nil || result.Extraction.State != "cancelled" {
		t.Fatal("cancelled results unavailable during transfer", result, err)
	}

	got, err := os.ReadFile(filepath.Join(volume, "world.bin"))
	if err != nil || !bytes.Equal(got, f.world) {
		t.Fatal("cancellation replaced existing world", err)
	}
}
