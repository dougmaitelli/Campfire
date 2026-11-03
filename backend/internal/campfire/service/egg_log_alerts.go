package service

import "campfire/internal/shared/model"

func eggFeatureLogAlert(feature string) (model.LogAlert, bool) {
	rule := model.LogAlert{Severity: model.LogError}

	switch feature {
	case "eula":
		rule.Title = "Minecraft EULA requires acceptance"
		rule.Message = "Read Minecraft's EULA. If you agree, set eula=true in eula.txt, save it, and start the server again."
		rule.Any = []model.LogMatch{{Value: "You need to agree to the EULA"}, {Value: "You need to agree to the Minecraft EULA"}}
		rule.Action = &model.LogAlertAction{Type: model.LogOpenFile, Path: "eula.txt"}
	case "java_version":
		rule.Title = "The server needs a different Java version"
		rule.Message = "Choose a Docker image with the Java version required by this server, then start it again."
		rule.Any = []model.LogMatch{{Value: "UnsupportedClassVersionError"}, {Value: "Unsupported major.minor version"}}
		rule.Action = &model.LogAlertAction{Type: model.LogServerSettings}
	case "pid_limit":
		rule.Title = "The server could not create a thread or process"
		rule.Message = "Check the server's memory usage and process limits on its Spark. This error can indicate insufficient memory or too many processes."
		rule.Any = []model.LogMatch{{Value: "unable to create native thread"}, {Value: "unable to create new native thread"}, {Value: "pthread_create failed (EAGAIN)"}}
	case "steam_disk_space":
		rule.Title = "SteamCMD ran out of disk space"
		rule.Message = "Free disk space on the Spark or increase the server's disk allocation, then retry installation or startup."
		rule.Any = []model.LogMatch{{Value: "Insufficient disk space"}, {Value: "No space left on device"}}
		rule.Action = &model.LogAlertAction{Type: model.LogServerSettings}
	default:
		return model.LogAlert{}, false
	}

	for index := range rule.Any {
		rule.Any[index].IgnoreCase = true
	}

	return rule, true
}
