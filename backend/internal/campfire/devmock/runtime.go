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
	"sync"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/protocol"

	"gorm.io/gorm"
)

// Runtime implements the panel's real dispatch interfaces without opening network connections.
type Runtime struct {
	db *gorm.DB
	mu sync.Mutex
}

type runtimeRecord struct {
	ServerID           string `gorm:"primaryKey"`
	SparkID            string
	Deployment         model.Deployment `gorm:"serializer:json"`
	State              string
	OperationID        string
	Operation          string
	Logs               string
	Files              map[string]string `gorm:"serializer:json"`
	FileFixtureVersion int               `gorm:"not null;default:0"`
}

func newRecord(spark string, deployment model.Deployment) runtimeRecord {
	return runtimeRecord{ServerID: deployment.ServerID, SparkID: spark, Deployment: deployment, State: "not_installed", Logs: "[Mock Spark] Configuration received. No Docker containers are used.\n", FileFixtureVersion: 1, Files: mockFiles()}
}

func mockFiles() map[string]string {
	return map[string]string{
		"server.properties":            "motd=Campfire mock server\nserver-port=25565\nmax-players=20\n",
		"eula.txt":                     "# Simulated fixture; no Minecraft software is running.\neula=false\n",
		"whitelist.json":               "[]\n",
		"ops.json":                     "[]\n",
		"config/":                      "",
		"config/fabric/":               "",
		"config/fabric/settings.json":  "{\n  \"debug\": false,\n  \"maxViewDistance\": 12\n}\n",
		"config/permissions.json":      "{\n  \"defaultRole\": \"player\"\n}\n",
		"logs/":                        "",
		"logs/latest.log":              "[12:00:00] [Server thread/INFO]: Starting mock Minecraft server\n[12:00:01] [Server thread/INFO]: Done! For help, type help\n",
		"mods/":                        "",
		"mods/README.txt":              "Place Fabric mod JARs here. This mock server does not execute them.\n",
		"world/":                       "",
		"world/datapacks/":             "",
		"world/datapacks/README.txt":   "Place world datapacks here.\n",
		"world/playerdata/":            "",
		"world/stats/":                 "",
		"world/stats/mock-player.json": "{\n  \"stats\": {\"minecraft:custom\": {\"minecraft:play_time\": 72000}},\n  \"DataVersion\": 4189\n}\n",
		"crash-reports/":               "",
	}
}

// Connected makes the dedicated offline fixture unavailable; newly created Sparks are simulated online.
func (r *Runtime) Connected(id string) bool { return id != "mock-offline" }

// SparkFeatures advertises only the transports this simulator actually implements.
func (r *Runtime) SparkFeatures(id string) (int, string, []string) {
	if !r.Connected(id) {
		return 0, "", nil
	}

	return protocol.ProtocolVersion, "mock-dev", []string{protocol.CapabilityConfigure, protocol.CapabilityLifecycle, protocol.CapabilityMetrics, protocol.CapabilityConsole, protocol.CapabilityFiles, protocol.CapabilityUpgrade, protocol.CapabilityDiskQuota, protocol.CapabilityBackups, protocol.CapabilityBackupDelete}
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
		record.State = "stopped"
	case "start", "restart":
		if record.State == "not_installed" || record.State == "deleted" {
			return model.RuntimeStatus{}, errors.New("install the mock server before starting it")
		}

		record.State = "running"
	case "stop", "kill":
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

		record.Logs += fmt.Sprintf("[Console] > %s\n[Mock server] Command received.\n", request.Content)
	default:
		return model.RuntimeStatus{}, fmt.Errorf("%s is not simulated in mock mode", request.Action)
	}

	if request.Action != "status" && request.Action != "logs" && request.Action != "metrics" {
		record.OperationID, record.Operation = request.OperationID, request.Action

		record.Logs += fmt.Sprintf("[Mock Spark] %s completed; state=%s\n", request.Action, record.State)
		if len(record.Logs) > 32000 {
			record.Logs = record.Logs[len(record.Logs)-16000:]
		}

		if err := r.db.WithContext(ctx).Save(&record).Error; err != nil {
			return model.RuntimeStatus{}, err
		}
	}

	status := model.RuntimeStatus{ServerID: record.ServerID, State: record.State, Ready: record.State == "running", OperationID: record.OperationID, Operation: record.Operation, DiskQuotaEnforced: quotaEnforced(record)}
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

	return &model.ResourceUsage{ServerID: record.ServerID, State: record.State, CPUPercent: &cpu, MemoryBytes: &memory, DiskBytes: &disk, SampledAt: now, DiskSampledAt: now, DiskQuotaEnforced: quotaEnforced(record)}
}

func quotaEnforced(record runtimeRecord) bool {
	return record.Deployment.Configuration.DiskMB > 0 && record.State != "not_installed" && record.State != "deleted"
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
