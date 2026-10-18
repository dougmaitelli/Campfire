package protocol

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// SparkUpdateTimeout bounds official binary download and verification.
const SparkUpdateTimeout = 5 * time.Minute

// CommandTimeout permits bounded archive work without enlarging other RPC budgets.
func CommandTimeout(command Command) time.Duration {
	if command.Name == UpdateSpark {
		return SparkUpdateTimeout
	}

	if command.Name == ServerFiles && command.Files != nil && (command.Files.Operation == "compress" || command.Files.Operation == "extract") {
		return 30 * time.Minute
	}

	return 30 * time.Second
}

// Capability names version the behavior of individual features independently.
const (
	CapabilityConfigure     = "server.configure.v1"
	CapabilityEmberBehavior = "server.ember.behavior.v1"
	CapabilityLifecycle     = "server.lifecycle.v1"
	CapabilityFiles         = "server.files.v1"
	CapabilityFileTransfer  = "server.files.transfer.v1"
	CapabilityFileArchive   = "server.files.archive.v1"
	CapabilityBackups       = "server.backups.v1"
	CapabilityBackupDelete  = "server.backups.delete.v1"
	CapabilityBackupRemote  = "server.backups.remote.v1"
	CapabilityMetrics       = "server.metrics.v1"
	CapabilityConsole       = "server.console.stream.v1"
	CapabilityDiskQuota     = "server.disk-quota.v1"
	CapabilityTransfer      = "server.transfer.v1"
	CapabilityUpgrade       = "server.upgrade.v1"
	CapabilityCleanInstall  = "server.reinstall.clean.v1"
	CapabilitySelfUpdate    = "spark.self-update.v1"
)

// SupportedCapabilities returns a fresh catalog of features implemented here.
func SupportedCapabilities() []string {
	return []string{CapabilityConfigure, CapabilityEmberBehavior, CapabilityLifecycle, CapabilityFiles, CapabilityFileTransfer, CapabilityFileArchive, CapabilityBackups, CapabilityBackupDelete, CapabilityBackupRemote, CapabilityMetrics, CapabilityConsole, CapabilityDiskQuota, CapabilityTransfer, CapabilityUpgrade, CapabilityCleanInstall, CapabilitySelfUpdate}
}

// ValidCapabilities bounds handshake input while allowing unknown future names.
func ValidCapabilities(items []string) bool {
	if len(items) > 64 {
		return false
	}

	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item == "" || len(item) > 64 || seen[item] {
			return false
		}

		for _, r := range item {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '.' && r != '-' && r != '_' {
				return false
			}
		}

		seen[item] = true
	}

	return true
}

// Negotiate selects the highest mutually supported wire version and only known,
// explicitly advertised capabilities. Missing capabilities never imply support.
func Negotiate(hello Message) (int, []string, error) {
	minimum := hello.MinimumProtocol
	if minimum == 0 {
		minimum = hello.Protocol
	}

	if minimum < 1 || hello.Protocol < minimum || hello.Protocol < MinimumProtocolVersion || minimum > ProtocolVersion || !ValidCapabilities(hello.Capabilities) {
		return 0, nil, errors.New("no compatible Spark protocol or invalid capability advertisement")
	}

	selected := make([]string, 0)

	for _, capability := range SupportedCapabilities() {
		if slices.Contains(hello.Capabilities, capability) {
			selected = append(selected, capability)
		}
	}

	slices.Sort(selected)

	return min(hello.Protocol, ProtocolVersion), selected, nil
}

// ValidateWelcome rejects unoffered versions or capabilities from the panel.
func ValidateWelcome(welcome Message, offered []string) error {
	if welcome.Type != "welcome" || welcome.Protocol < MinimumProtocolVersion || welcome.Protocol > ProtocolVersion || !ValidCapabilities(welcome.Capabilities) {
		return errors.New("invalid panel capability negotiation")
	}

	for _, capability := range welcome.Capabilities {
		if !slices.Contains(offered, capability) {
			return errors.New("panel selected an unoffered capability")
		}
	}

	return nil
}

// Require returns an explicit feature error rather than sending unsupported work.
func Require(negotiated []string, required ...string) error {
	for _, capability := range required {
		if !slices.Contains(negotiated, capability) {
			return &CommandError{Code: "unsupported_capability", Message: "Spark does not support required capability " + capability + ". Update the Spark or use a compatible node."}
		}
	}

	return nil
}

// RequiredCapabilities covers every command extension at both dispatch boundaries.
func RequiredCapabilities(command Command) []string {
	switch command.Name {
	case UpdateSpark:
		return []string{CapabilitySelfUpdate}
	case ConfigureServer:
		if command.Deployment != nil && command.Deployment.Ember.ExtendedBehavior() {
			return []string{CapabilityConfigure, CapabilityEmberBehavior}
		}

		return []string{CapabilityConfigure}
	case ServerTransfer:
		return []string{CapabilityTransfer, CapabilityDiskQuota}
	case ServerFiles:
		if command.Files != nil {
			switch command.Files.Operation {
			case "stat", "read.chunk", "upload.begin", "upload.chunk", "upload.commit", "upload.cancel":
				return []string{CapabilityFiles, CapabilityFileTransfer}
			case "compress", "extract":
				return []string{CapabilityFiles, CapabilityFileArchive}
			}
		}

		if command.Files != nil && strings.HasPrefix(command.Files.Operation, "backup.") {
			if command.Files.Operation == "backup.delete" {
				return []string{CapabilityBackups, CapabilityBackupDelete}
			}

			return []string{CapabilityBackups}
		}

		return []string{CapabilityFiles}
	case ServerAction:
		requirements := []string{CapabilityLifecycle}
		if command.Runtime == nil {
			return requirements
		}

		switch command.Runtime.Action {
		case "metrics":
			return []string{CapabilityMetrics}
		case "backup":
			requirements = append(requirements, CapabilityBackups)
		case "restore":
			requirements = append(requirements, CapabilityBackups, CapabilityDiskQuota)
			if command.Runtime.BackupSource != nil {
				requirements = append(requirements, CapabilityBackupRemote)
			}
		case "update", "reinstall":
			requirements = append(requirements, CapabilityUpgrade, CapabilityDiskQuota)
			if command.Runtime.Action == "reinstall" && command.Runtime.Content == "clean" {
				requirements = append(requirements, CapabilityCleanInstall)
			}
		case "install", "start", "restart":
			requirements = append(requirements, CapabilityDiskQuota)
		}

		return requirements
	default:
		return nil
	}
}
