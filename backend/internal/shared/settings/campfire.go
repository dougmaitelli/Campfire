// Package settings defines the configuration names shared by application entry points.
package settings

import "campfire/internal/shared/cliutil"

// Campfire settings are shared by panel startup and administrator recovery.
var (
	CampfireListen                  = cliutil.Setting{FlagName: "listen", EnvironmentName: "CAMPFIRE_LISTEN"}
	CampfireDatabase                = cliutil.Setting{FlagName: "database", EnvironmentName: "CAMPFIRE_DATABASE"}
	CampfireData                    = cliutil.Setting{FlagName: "data-dir", EnvironmentName: "CAMPFIRE_DATA"}
	CampfireSFTPListen              = cliutil.Setting{FlagName: "sftp-listen", EnvironmentName: "CAMPFIRE_SFTP_LISTEN"}
	CampfireSFTPHostKey             = cliutil.Setting{FlagName: "sftp-host-key", EnvironmentName: "CAMPFIRE_SFTP_HOST_KEY"}
	CampfireMaxUploadMiB            = cliutil.Setting{FlagName: "max-upload-mib", EnvironmentName: "CAMPFIRE_MAX_UPLOAD_MIB"}
	CampfirePublicURL               = cliutil.Setting{FlagName: "public-url", EnvironmentName: "CAMPFIRE_PUBLIC_URL"}
	CampfireOIDCIssuer              = cliutil.Setting{FlagName: "oidc-issuer", EnvironmentName: "CAMPFIRE_OIDC_ISSUER"}
	CampfireOIDCClientID            = cliutil.Setting{FlagName: "oidc-client-id", EnvironmentName: "CAMPFIRE_OIDC_CLIENT_ID"}
	CampfireOIDCClientSecret        = cliutil.Setting{FlagName: "oidc-client-secret", EnvironmentName: "CAMPFIRE_OIDC_CLIENT_SECRET"}
	CampfireOIDCAutoLinkEmail       = cliutil.Setting{FlagName: "oidc-auto-link-email", EnvironmentName: "CAMPFIRE_OIDC_AUTO_LINK_EMAIL"}
	CampfireDisableLocalLogin       = cliutil.Setting{FlagName: "disable-local-login", EnvironmentName: "CAMPFIRE_DISABLE_LOCAL_LOGIN"}
	CampfireAgentListen             = cliutil.Setting{FlagName: "agent-listen", EnvironmentName: "CAMPFIRE_AGENT_LISTEN"}
	CampfireAgentPublicURL          = cliutil.Setting{FlagName: "agent-public-url", EnvironmentName: "CAMPFIRE_AGENT_PUBLIC_URL"}
	CampfireAgentTLSCert            = cliutil.Setting{FlagName: "agent-tls-cert", EnvironmentName: "CAMPFIRE_AGENT_TLS_CERT"}
	CampfireAgentTLSKey             = cliutil.Setting{FlagName: "agent-tls-key", EnvironmentName: "CAMPFIRE_AGENT_TLS_KEY"}
	CampfireAuditRetentionDays      = cliutil.Setting{FlagName: "audit-retention-days", EnvironmentName: "CAMPFIRE_AUDIT_RETENTION_DAYS"}
	CampfireOperationRetentionDays  = cliutil.Setting{FlagName: "operation-retention-days", EnvironmentName: "CAMPFIRE_OPERATION_RETENTION_DAYS"}
	CampfireGameEventRetentionDays  = cliutil.Setting{FlagName: "game-event-retention-days", EnvironmentName: "CAMPFIRE_GAME_EVENT_RETENTION_DAYS"}
	CampfireGameEventLimit          = cliutil.Setting{FlagName: "game-event-limit", EnvironmentName: "CAMPFIRE_GAME_EVENT_LIMIT"}
	CampfireResourceIntervalSeconds = cliutil.Setting{EnvironmentName: "CAMPFIRE_RESOURCE_INTERVAL_SECONDS"}
	CampfireResourceRetentionDays   = cliutil.Setting{EnvironmentName: "CAMPFIRE_RESOURCE_RETENTION_DAYS"}
	CampfireAlertCPUPercent         = cliutil.Setting{EnvironmentName: "CAMPFIRE_ALERT_CPU_PERCENT"}
	CampfireAlertMemoryPercent      = cliutil.Setting{EnvironmentName: "CAMPFIRE_ALERT_MEMORY_PERCENT"}
	CampfireAlertDiskPercent        = cliutil.Setting{EnvironmentName: "CAMPFIRE_ALERT_DISK_PERCENT"}
	CampfireAlertDurationSeconds    = cliutil.Setting{EnvironmentName: "CAMPFIRE_ALERT_DURATION_SECONDS"}
	CampfireAlertHealthEnabled      = cliutil.Setting{EnvironmentName: "CAMPFIRE_ALERT_HEALTH_ENABLED"}
	CampfireAppriseURL              = cliutil.Setting{EnvironmentName: "CAMPFIRE_APPRISE_URL"}
	CampfireAppriseURLs             = cliutil.Setting{EnvironmentName: "CAMPFIRE_APPRISE_URLS"}
	CampfireAppriseTags             = cliutil.Setting{EnvironmentName: "CAMPFIRE_APPRISE_TAGS"}
	CampfireS3Bucket                = cliutil.Setting{EnvironmentName: "CAMPFIRE_S3_BUCKET"}
	CampfireS3Region                = cliutil.Setting{EnvironmentName: "CAMPFIRE_S3_REGION"}
	CampfireS3Endpoint              = cliutil.Setting{EnvironmentName: "CAMPFIRE_S3_ENDPOINT"}
	CampfireS3Prefix                = cliutil.Setting{EnvironmentName: "CAMPFIRE_S3_PREFIX"}
	CampfireS3PathStyle             = cliutil.Setting{EnvironmentName: "CAMPFIRE_S3_PATH_STYLE"}
	RecoveryPasswordFile            = cliutil.Setting{FlagName: "password-file"}
	RecoveryUsername                = cliutil.Setting{FlagName: "username"}
	Version                         = cliutil.Setting{FlagName: "version"}
)
