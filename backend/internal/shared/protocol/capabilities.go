package protocol

import (
	"errors"
	"slices"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/utils"
)

// SparkUpdateTimeout bounds panel-hosted binary download and verification.
const SparkUpdateTimeout = 5 * time.Minute

// SparkPruneTimeout bounds explicit node cleanup.
const SparkPruneTimeout = 5 * time.Minute

// CommandTimeout permits bounded archive work without enlarging other RPC budgets.
func CommandTimeout(command Command) time.Duration {
	if command.Name == PruneSpark {
		return SparkPruneTimeout
	}

	if command.Name == UpdateSpark || command.Name == ReinstallSpark {
		return SparkUpdateTimeout
	}

	if command.Name == ServerFiles && command.Files != nil && (command.Files.Operation == model.FileOperationCompress || command.Files.Operation == model.FileOperationExtract) {
		return 30 * time.Minute
	}

	return 30 * time.Second
}

// Capability names version individual feature payloads and behavior independently
// of the wire protocol. Incompatible feature changes require a new capability
// version; only explicitly negotiated names are supported.
const (
	CapabilityConfigure       = "server.configure.v2"
	CapabilityLifecycle       = "server.lifecycle.v1"
	CapabilityFiles           = "server.files.v1"
	CapabilityFilePermissions = "server.files.permissions.v1"
	CapabilityFileTransfer    = "server.files.transfer.v1"
	CapabilityFileArchive     = "server.files.archive.v1"
	CapabilityBackups         = "server.backups.v1"
	CapabilityBackupRemote    = "server.backups.remote.v1"
	CapabilityMetrics         = "server.metrics.v1"
	CapabilityConsole         = "server.console.stream.v1"
	CapabilityDiskQuota       = "server.disk-quota.v1"
	CapabilityTransfer        = "server.transfer.v1"
	CapabilityReinstall       = "server.reinstall.v1"
	CapabilitySelfUpdate      = "spark.self-update.v1"
	CapabilitySelfReinstall   = "spark.self-reinstall.v1"
	CapabilityPrune           = "spark.prune.v1"
	CapabilityGameIntegration = "server.game-integration.v1"
	CapabilityGameEvents      = "server.game-events.v1"
	CapabilityGameMedia       = "server.game-media.v1"
)

// SupportedCapabilities returns a fresh catalog of features implemented here.
func SupportedCapabilities() []string {
	return []string{CapabilityConfigure, CapabilityLifecycle, CapabilityFiles, CapabilityFileTransfer, CapabilityFileArchive, CapabilityFilePermissions, CapabilityBackups, CapabilityBackupRemote, CapabilityMetrics, CapabilityConsole, CapabilityDiskQuota, CapabilityTransfer, CapabilityReinstall, CapabilitySelfUpdate, CapabilitySelfReinstall, CapabilityPrune, CapabilityGameIntegration, CapabilityGameEvents, CapabilityGameMedia}
}

// ValidCapabilities bounds handshake input while allowing unknown future names.
func ValidCapabilities(items []string) bool {
	if len(items) > 64 {
		return false
	}

	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if len(item) > 64 || seen[item] || !utils.IsValidIdentifier(item) {
			return false
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
	if welcome.Type != MessageTypeWelcome || welcome.Protocol < MinimumProtocolVersion || welcome.Protocol > ProtocolVersion || !ValidCapabilities(welcome.Capabilities) {
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

var commandCapabilities = map[CommandName][]string{
	PruneSpark:      {CapabilityPrune},
	UpdateSpark:     {CapabilitySelfUpdate},
	ReinstallSpark:  {CapabilitySelfReinstall},
	ConfigureServer: {CapabilityConfigure},
	ServerTransfer:  {CapabilityTransfer},
	ServerFiles:     {CapabilityFiles},
	ServerAction:    {CapabilityLifecycle},
}

var fileOperationCapabilities = map[model.FileOperation]string{
	model.FileOperationChmod:        CapabilityFilePermissions,
	model.FileOperationStat:         CapabilityFileTransfer,
	model.FileOperationReadChunk:    CapabilityFileTransfer,
	model.FileOperationUploadBegin:  CapabilityFileTransfer,
	model.FileOperationUploadChunk:  CapabilityFileTransfer,
	model.FileOperationUploadCommit: CapabilityFileTransfer,
	model.FileOperationUploadCancel: CapabilityFileTransfer,
	model.FileOperationCompress:     CapabilityFileArchive,
	model.FileOperationExtract:      CapabilityFileArchive,
}

var runtimeActionCapabilities = map[model.ServerAction][]string{
	model.ServerActionMetrics:   {CapabilityMetrics},
	model.ServerActionBackup:    {CapabilityLifecycle, CapabilityBackups},
	model.ServerActionRestore:   {CapabilityLifecycle, CapabilityBackups},
	model.ServerActionUpdate:    {CapabilityLifecycle, CapabilityReinstall},
	model.ServerActionReinstall: {CapabilityLifecycle, CapabilityReinstall},
}

// RequiredCapabilities covers every command extension at both dispatch boundaries.
func RequiredCapabilities(command Command) []string {
	required := slices.Clone(commandCapabilities[command.Name])
	if command.Name == ConfigureServer && command.Deployment != nil && command.Deployment.Configuration.GameIntegration {
		required = append(required, CapabilityGameIntegration)
	}

	if command.Name == ServerFiles && command.Files != nil {
		if command.Files.Operation.IsBackup() {
			return []string{CapabilityBackups}
		}

		if capability, ok := fileOperationCapabilities[command.Files.Operation]; ok {
			required = append(required, capability)
		}
	}

	if command.Name == ServerAction && command.Runtime != nil {
		if capabilities, ok := runtimeActionCapabilities[command.Runtime.Action]; ok {
			required = slices.Clone(capabilities)
		}

		if command.Runtime.Action == model.ServerActionRestore && command.Runtime.BackupSource != nil {
			required = append(required, CapabilityBackupRemote)
		}
	}

	return required
}
