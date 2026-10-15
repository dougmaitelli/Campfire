package cli

import (
	"errors"
	"strconv"

	"campfire/internal/shared/model"
)

func parseMaxUpload(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 || value > model.FileTransferLimit/(1<<20) {
		return 0, errors.New("CAMPFIRE_MAX_UPLOAD_MIB must be an integer between 1 and 102400")
	}

	return value * (1 << 20), nil
}
