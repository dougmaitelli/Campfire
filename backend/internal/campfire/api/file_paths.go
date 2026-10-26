package api

import (
	"io/fs"
	"strings"
	"unicode"

	"campfire/internal/shared/model"
)

// relativeDataPath translates the public server-data namespace into the Spark
// protocol's relative paths. Validate before any cleaning can hide traversal.
func relativeDataPath(value string) (string, error) {
	if len(value) > 1026 || strings.ContainsAny(value, "\\\x00") || strings.ContainsFunc(value, unicode.IsControl) {
		return "", &model.FileError{Code: "invalid_file", Message: "Use a path within the server data directory."}
	}

	value = strings.Trim(value, "/")
	if value == "" {
		return ".", nil
	}

	if len(value) > 1024 || !fs.ValidPath(value) {
		return "", &model.FileError{Code: "invalid_file", Message: "Use a path within the server data directory."}
	}

	return value, nil
}

func normalizeFileRequest(input *model.FileRequest) error {
	var err error

	input.Path, err = relativeDataPath(input.Path)
	if err != nil {
		return err
	}

	if input.Destination != "" {
		input.Destination, err = relativeDataPath(input.Destination)
		if err != nil {
			return err
		}
	}

	for index, value := range input.Paths {
		input.Paths[index], err = relativeDataPath(value)
		if err != nil {
			return err
		}
	}

	return nil
}

func publicDataPath(value string, directory bool) string {
	if value == "" || value == "." {
		return "/"
	}

	value = "/" + strings.Trim(value, "/")
	if directory {
		value += "/"
	}

	return value
}
