//go:build !production

package devmock

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"

	"gorm.io/gorm"
)

func mockTransferService(t *testing.T, runtime *Runtime, db *database.Database) *service.TransferService {
	t.Helper()

	embers := service.NewEmberService(repository.NewEmberRepository(db.ORM()))
	servers := service.NewServerService(repository.NewServerRepository(db.ORM()), embers)
	deployment := &service.DeploymentService{Servers: servers, Embers: embers, Remote: runtime, Sparks: service.NewSparkService(repository.NewSparkRepository(db.ORM()), runtime)}
	transfers := service.NewTransferService(repository.NewTransferRepository(db.ORM()), deployment)
	deployment.Transfers = transfers
	servers.WithRuntimeGuard(deployment)

	return transfers
}

func waitMockTransfer(t *testing.T, transfers *service.TransferService, id string, phase model.TransferPhase) database.ServerTransfer {
	t.Helper()

	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		item, err := transfers.Store.Get(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}

		if item.Phase == phase {
			return item
		}

		if item.Phase == model.TransferPhaseFailed && phase != model.TransferPhaseFailed {
			t.Fatal("mock transfer failed", item.Error)
		}

		select {
		case <-deadline.C:
			t.Fatal("mock transfer did not reach phase", phase, item)
		case <-ticker.C:
		}
	}
}

type interruptedTransfer struct {
	*Runtime
	writes int
}

func (r *interruptedTransfer) Transfer(ctx context.Context, spark string, request model.TransferRequest) (model.TransferResult, error) {
	if request.Operation == model.TransferOperationWrite {
		r.writes++
		if r.writes == 4 {
			return model.TransferResult{}, errors.New("mock connection interrupted")
		}
	}

	return r.Runtime.Transfer(ctx, spark, request)
}

func TestMockTransferMovesExistingServerAndResumesAfterRestart(t *testing.T) {
	runtime, db, auth, servers := fixture(t)
	server := onlineServer(t, runtime, servers)
	transfers := mockTransferService(t, runtime, db)

	if _, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: model.ServerActionKill}); err != nil {
		t.Fatal(err)
	}

	original, err := runtime.load(t.Context(), server.SparkID, server.ID)
	if err != nil {
		t.Fatal(err)
	}

	original.Files["world/save.dat"] = "saved world data"
	original.FileModes = map[string]uint32{"world/save.dat": 0o640}

	if err := db.ORM().Save(&original).Error; err != nil {
		t.Fatal(err)
	}

	var backups []backupRecord
	if err := db.ORM().Where("server_id = ?", server.ID).Find(&backups).Error; err != nil || len(backups) == 0 {
		t.Fatal("missing backup fixtures", err)
	}

	transfers.Deployment.Remote = &interruptedTransfer{Runtime: runtime}

	item, err := transfers.Begin(t.Context(), server.ID, "mock-america", server.Ports)
	if err != nil {
		t.Fatal(err)
	}

	transfers.Start(t.Context())
	t.Cleanup(transfers.Close)
	failed := waitMockTransfer(t, transfers, item.ID, model.TransferPhaseFailed)
	transfers.Close()

	if failed.ResumePhase != model.TransferPhaseCopying || failed.TransferredBytes == 0 || failed.TransferredBytes >= failed.TotalBytes {
		t.Fatal("expected a resumable partial copy", failed)
	}

	unchanged, err := transfers.Deployment.Servers.Get(t.Context(), server.ID)
	if err != nil || unchanged.SparkID != server.SparkID {
		t.Fatal("failed copy changed placement", unchanged, err)
	}

	if source, err := runtime.load(t.Context(), server.SparkID, server.ID); err != nil || !maps.Equal(source.Files, original.Files) {
		t.Fatal("staging overwrote source files", err)
	}

	reloaded, err := New(t.Context(), db.ORM(), auth)
	if err != nil {
		t.Fatal(err)
	}

	transfers = mockTransferService(t, reloaded, db)
	if err := transfers.Store.Retry(t.Context(), item.ID); err != nil {
		t.Fatal(err)
	}

	transfers.Start(t.Context())
	t.Cleanup(transfers.Close)
	complete := waitMockTransfer(t, transfers, item.ID, model.TransferPhaseComplete)

	if complete.Active || complete.TransferredBytes != complete.TotalBytes {
		t.Fatal("completed transfer has incomplete progress", complete)
	}

	moved, err := transfers.Deployment.Servers.Get(t.Context(), server.ID)
	if err != nil || moved.SparkID != "mock-america" || moved.OwnerUserID != server.OwnerUserID {
		t.Fatal("placement or ownership was not preserved", moved, err)
	}

	destination, err := reloaded.load(t.Context(), moved.SparkID, server.ID)
	if err != nil || destination.State != model.RuntimeStateStopped || !maps.Equal(destination.Files, original.Files) || !maps.Equal(destination.FileModes, original.FileModes) {
		t.Fatal("destination lost runtime data", destination, err)
	}

	if _, err := reloaded.load(t.Context(), server.SparkID, server.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("source still exposes the transferred runtime", err)
	}

	var retained []backupRecord
	if err := db.ORM().Where("server_id = ?", server.ID).Find(&retained).Error; err != nil || len(retained) != len(backups) {
		t.Fatal("completed backups were lost", retained, err)
	}

	if _, err := transfers.Deployment.Action(t.Context(), server.ID, model.ServerActionStart); err != nil {
		t.Fatal("transferred server cannot start", err)
	}
}

func TestMockTransferCancellationPreservesSource(t *testing.T) {
	runtime, db, _, servers := fixture(t)
	server := onlineServer(t, runtime, servers)
	transfers := mockTransferService(t, runtime, db)

	if _, err := runtime.Action(t.Context(), server.SparkID, model.RuntimeRequest{ServerID: server.ID, Action: model.ServerActionKill}); err != nil {
		t.Fatal(err)
	}

	item, err := transfers.Begin(t.Context(), server.ID, "mock-america", server.Ports)
	if err != nil {
		t.Fatal(err)
	}

	transfers.Start(t.Context())
	t.Cleanup(transfers.Close)
	waitMockTransfer(t, transfers, item.ID, model.TransferPhaseCopying)

	if err := transfers.Store.Cancel(t.Context(), item.ID); err != nil {
		t.Fatal(err)
	}

	waitMockTransfer(t, transfers, item.ID, model.TransferPhaseCancelled)

	unchanged, err := transfers.Deployment.Servers.Get(t.Context(), server.ID)
	if err != nil || unchanged.SparkID != server.SparkID {
		t.Fatal("cancelled transfer changed placement", unchanged, err)
	}

	if _, err := runtime.load(t.Context(), server.SparkID, server.ID); err != nil {
		t.Fatal("cancelled transfer lost source", err)
	}

	var staging int64
	if err := db.ORM().Model(&transferDeployment{}).Where("server_id = ?", server.ID).Count(&staging).Error; err != nil || staging != 0 {
		t.Fatal("cancelled destination was not discarded", staging, err)
	}

	if _, err := transfers.Begin(t.Context(), server.ID, "mock-america", server.Ports); err != nil {
		t.Fatal("cancellation did not release reservations", err)
	}
}

func TestMockTransferCapabilitiesAndGuards(t *testing.T) {
	runtime, db, _, servers := fixture(t)
	server := onlineServer(t, runtime, servers)
	transfers := mockTransferService(t, runtime, db)

	_, _, features := runtime.SparkFeatures(server.SparkID)
	if !slices.Contains(features, protocol.CapabilityTransfer) {
		t.Fatal("online mock Spark does not advertise transfers")
	}

	if _, err := transfers.Begin(t.Context(), server.ID, "mock-america", server.Ports); !errors.Is(err, service.ErrServerActive) {
		t.Fatal("running source was accepted", err)
	}

	if _, err := transfers.Begin(t.Context(), server.ID, "mock-offline", server.Ports); !errors.Is(err, service.ErrDeploymentUnavailable) {
		t.Fatal("offline destination was accepted", err)
	}
}
