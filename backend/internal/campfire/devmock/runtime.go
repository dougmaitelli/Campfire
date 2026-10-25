//go:build !production

package devmock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"campfire/internal/campfire/devfixture"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"

	"gorm.io/gorm"
)

// Runtime implements the panel's real dispatch interfaces without opening network connections.
type Runtime struct {
	db           *gorm.DB
	mu           sync.Mutex
	games        *service.GameIntegrationService
	gameSessions map[string]*gameSession
}

type runtimeRecord struct {
	ServerID           string `gorm:"primaryKey"`
	SparkID            string
	Deployment         model.Deployment `gorm:"serializer:json"`
	State              string
	StartedAt          int64 `gorm:"not null;default:0"`
	StoppedAt          int64 `gorm:"not null;default:0"`
	OperationID        string
	Operation          string
	Logs               string
	Files              map[string]string `gorm:"serializer:json"`
	FileFixtureVersion int               `gorm:"not null;default:0"`
}

func newRecord(spark string, deployment model.Deployment) runtimeRecord {
	return runtimeRecord{ServerID: deployment.ServerID, SparkID: spark, Deployment: deployment, State: "not_installed", Logs: timestampLog("[Mock Spark] Configuration received. No Docker containers are used.\n"), FileFixtureVersion: 1, Files: mockFiles()}
}

func timestampLog(content string) string {
	prefix := time.Now().UTC().Format(time.RFC3339Nano) + " "
	return prefix + strings.ReplaceAll(strings.TrimSuffix(content, "\n"), "\n", "\n"+prefix) + "\n"
}

func mockFiles() map[string]string { return devfixture.Files() }

// Connected makes the dedicated offline fixture unavailable; newly created Sparks are simulated online.
func (r *Runtime) Connected(id string) bool { return id != "mock-offline" }

// SparkFeatures advertises only the transports this simulator actually implements.
func (r *Runtime) SparkFeatures(id string) (int, string, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.Connected(id) {
		return 0, "", nil
	}

	version := "mock-dev"

	var client sparkClientRecord
	if err := r.db.Where("spark_id = ?", id).Take(&client).Error; err == nil {
		version = client.Version
	}

	features := []string{protocol.CapabilityConfigure, protocol.CapabilityLifecycle, protocol.CapabilityMetrics, protocol.CapabilityConsole, protocol.CapabilityFiles, protocol.CapabilityReinstall, protocol.CapabilityDiskQuota, protocol.CapabilityBackups, protocol.CapabilitySelfUpdate, protocol.CapabilityPrune}
	features = append(features, protocol.CapabilityStartupOverride)
	if r.games != nil {
		features = append(features, protocol.CapabilityGameIntegration, protocol.CapabilityGameEvents)
		if r.games.Media != nil {
			features = append(features, protocol.CapabilityGameMedia)
		}
	}

	if id == "mock-america" {
		features = slices.DeleteFunc(features, func(feature string) bool { return feature == protocol.CapabilityDiskQuota })
	}

	return protocol.ProtocolVersion, version, features
}

func (r *Runtime) load(ctx context.Context, spark, id string) (runtimeRecord, error) {
	if !r.Connected(spark) {
		return runtimeRecord{}, errors.New("mock Spark is offline")
	}

	var record runtimeRecord

	err := r.db.WithContext(ctx).Where("server_id = ? AND spark_id = ?", id, spark).Take(&record).Error

	return record, err
}

// Deploy durably records desired configuration, preserving the simulated runtime state.
func (r *Runtime) Deploy(ctx context.Context, spark string, deployment model.Deployment) (model.ConfigurationReceipt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, err := r.load(ctx, spark, deployment.ServerID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		record = newRecord(spark, deployment)
	} else if err != nil {
		return model.ConfigurationReceipt{}, err
	}

	record.Deployment = deployment
	if err := r.db.WithContext(ctx).Save(&record).Error; err != nil {
		return model.ConfigurationReceipt{}, err
	}

	encoded, err := json.Marshal(deployment)
	if err != nil {
		return model.ConfigurationReceipt{}, err
	}

	revision := sha256.Sum256(encoded)

	return model.ConfigurationReceipt{ServerID: deployment.ServerID, Revision: hex.EncodeToString(revision[:])}, nil
}

// Action simulates lifecycle commands and reports fresh, repeatable resource readings.
func (r *Runtime) Action(ctx context.Context, spark string, request model.RuntimeRequest) (model.RuntimeStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, err := r.load(ctx, spark, request.ServerID)
	if err != nil {
		return model.RuntimeStatus{}, err
	}

	switch request.Action {
	case "status", "logs", "metrics":
	case "install", "update", "reinstall":
		if request.Action == "reinstall" && request.Content == "clean" {
			record.Files = mockFiles()
		}

		record.State = "stopped"
	case "start", "restart":
		if record.State == "not_installed" || record.State == "deleted" {
			return model.RuntimeStatus{}, errors.New("install the mock server before starting it")
		}

		record.State = "running"
		record.StartedAt, record.StoppedAt = time.Now().Unix(), 0
	case "stop", "kill":
		if record.State == "running" {
			record.StoppedAt = time.Now().Unix()
		}

		record.State = "stopped"
	case "backup", "restore":
		if err := r.backupAction(ctx, &record, request); err != nil {
			return model.RuntimeStatus{}, err
		}
	case "delete":
		record.State = "deleted"
	case "command":
		if record.State != "running" {
			return model.RuntimeStatus{}, errors.New("mock server is not running")
		}

		record.Logs += timestampLog(fmt.Sprintf("[Console] > %s\n[Mock server] Command received.\n", request.Content))
		if err := r.gameCommand(ctx, record, request.Content); err != nil {
			return model.RuntimeStatus{}, err
		}
	default:
		return model.RuntimeStatus{}, fmt.Errorf("%s is not simulated in mock mode", request.Action)
	}

	if request.Action != "status" && request.Action != "logs" && request.Action != "metrics" {
		record.OperationID, record.Operation = request.OperationID, request.Action

		record.Logs += timestampLog(fmt.Sprintf("[Mock Spark] %s completed; state=%s\n", request.Action, record.State))
		if len(record.Logs) > 32000 {
			record.Logs = record.Logs[len(record.Logs)-16000:]
		}

		if err := r.db.WithContext(ctx).Save(&record).Error; err != nil {
			return model.RuntimeStatus{}, err
		}

		if request.Action != "command" && request.Action != "backup" {
			delete(r.gameSessions, record.ServerID)
		}
	}

	status := model.RuntimeStatus{ServerID: record.ServerID, State: record.State, Ready: record.State == "running", OperationID: record.OperationID, Operation: record.Operation, DiskQuotaEnforced: quotaEnforced(record)}
	if record.OperationID != "" {
		status.OperationState = model.OperationSucceeded
		status.Progress = "Complete"
	}

	if record.SparkID == "mock-america" && record.Deployment.Configuration.DiskMB > 0 {
		status.DiskQuotaErrorCode = model.RuntimeQuotaUnavailable
		status.DiskQuotaError = "Hard disk quotas are unavailable on this mock Spark. Game-data usage is still monitored."
	}

	if request.Action == "logs" {
		status.Logs = record.Logs
	}

	if request.Action == "metrics" {
		status.Metrics = metrics(record)
	}

	return status, nil
}

func metrics(record runtimeRecord) *model.ResourceUsage {
	now := time.Now().Unix()
	cpu := 0.0
	memory := int64(0)
	disk := diskUsage(record)

	if record.State == "running" {
		cpu = 35 + 15*math.Sin(float64(now%360)/12)
		memory = record.Deployment.Configuration.MemoryMB * (1 << 20) / 3
	}

	return &model.ResourceUsage{ServerID: record.ServerID, State: record.State, StartedAt: record.StartedAt, StoppedAt: record.StoppedAt, CPUPercent: &cpu, MemoryBytes: &memory, DiskBytes: &disk, SampledAt: now, DiskSampledAt: now, DiskQuotaEnforced: quotaEnforced(record)}
}

func quotaEnforced(record runtimeRecord) bool {
	return record.SparkID != "mock-america" && record.Deployment.Configuration.DiskMB > 0 && record.State != "not_installed" && record.State != "deleted"
}

func diskUsage(record runtimeRecord) int64 {
	// Simulate installed game data plus editable files, without consuming host disk.
	size := record.Deployment.Configuration.DiskMB * (1 << 20) / 40
	for _, data := range record.Files {
		size += int64(len(data))
	}

	return size
}

// Console streams saved output and subsequent command responses until the client disconnects.
func (r *Runtime) Console(ctx context.Context, spark, id string, emit func(model.ConsoleEvent) error) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	previous := ""
	connected := false

	for {
		status, err := r.Action(ctx, spark, model.RuntimeRequest{ServerID: id, Action: "logs"})
		if err != nil {
			return err
		}

		if status.Logs != previous {
			content := status.Logs
			if len(previous) <= len(content) && content[:len(previous)] == previous {
				content = content[len(previous):]
			}

			if err := emit(model.ConsoleEvent{Content: content}); err != nil {
				return err
			}

			previous = status.Logs
		}

		if !connected {
			if err := emit(model.ConsoleEvent{Ready: true}); err != nil {
				return err
			}

			connected = true
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
