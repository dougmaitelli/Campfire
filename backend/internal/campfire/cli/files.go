package cli

import (
	"fmt"
	"strconv"

	"campfire/internal/shared/model"
	"campfire/internal/shared/settings"
)

func parseMaxUpload(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 || value > model.FileTransferLimit/(1<<20) {
		return 0, fmt.Errorf("%s must be an integer between 1 and 102400", settings.CampfireMaxUploadMiB.EnvironmentName)
	}

	return value * (1 << 20), nil
}
