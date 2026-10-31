package protocol

// CommandErrorCode identifies a rejected or failed Spark command.
type CommandErrorCode string

// Command errors retain stable wire values across panel and Spark.
const (
	ErrorUnsupportedCapability  CommandErrorCode = "unsupported_capability"
	ErrorInvalidDeadline        CommandErrorCode = "invalid_deadline"
	ErrorPruneFailed            CommandErrorCode = "spark_prune_failed"
	ErrorUpdateFailed           CommandErrorCode = "spark_update_failed"
	ErrorInvalidTransfer        CommandErrorCode = "invalid_transfer"
	ErrorTransferFailed         CommandErrorCode = "transfer_failed"
	ErrorInvalidOperation       CommandErrorCode = "invalid_operation"
	ErrorRuntimeRejected        CommandErrorCode = "runtime_rejected"
	ErrorUnsupportedCommand     CommandErrorCode = "unsupported_command"
	ErrorInvalidConfiguration   CommandErrorCode = "invalid_configuration"
	ErrorConfigurationRejected  CommandErrorCode = "configuration_rejected"
	ErrorPublicationRejected    CommandErrorCode = "publication_rejected"
	ErrorInvalidPublication     CommandErrorCode = "invalid_publication"
	ErrorIntegrationUnavailable CommandErrorCode = "integration_unavailable"
	ErrorIngestionUnavailable   CommandErrorCode = "ingestion_unavailable"
)
