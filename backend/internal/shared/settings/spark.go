package settings

import "campfire/internal/shared/cliutil"

// Spark settings include enrollment, agent state, and the Docker endpoint.
var (
	SparkPanel            = cliutil.Setting{FlagName: "panel", EnvironmentName: "SPARK_PANEL"}
	SparkData             = cliutil.Setting{FlagName: "data", EnvironmentName: "SPARK_DATA"}
	SparkCAFile           = cliutil.Setting{FlagName: "ca-file", EnvironmentName: "SPARK_CA_FILE"}
	SparkTokenFile        = cliutil.Setting{FlagName: "token-file", EnvironmentName: "SPARK_TOKEN_FILE"}
	SparkEnrollmentToken  = cliutil.Setting{EnvironmentName: "CAMPFIRE_ENROLLMENT_TOKEN"}
	SparkUpdateInfo       = cliutil.Setting{FlagName: "update-info"}
	SparkEnrollOnly       = cliutil.Setting{FlagName: "enroll-only"}
	SparkGameIntegrations = cliutil.Setting{FlagName: "game-integrations"}
	DockerContext         = cliutil.Setting{EnvironmentName: "DOCKER_CONTEXT"}
	DockerHost            = cliutil.Setting{EnvironmentName: "DOCKER_HOST"}
)
