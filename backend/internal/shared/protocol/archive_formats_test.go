package protocol

import (
	"slices"
	"testing"

	"campfire/internal/shared/model"
)

func TestAdditionalArchiveFormatsRequireNegotiationOnlyAtStart(t *testing.T) {
	for _, operation := range []model.FileOperation{model.FileOperationExtract, model.FileOperationExtractStart, model.FileOperationCompress, model.FileOperationCompressStart} {
		for _, item := range []struct {
			name     string
			expanded bool
		}{
			{"save.tar.gz", false}, {"save.zip", true}, {"save.tgz", true}, {"save.tar", true}, {"save.tar.xz", true}, {"save.tar.lz", true}, {"save.tar.bz2", true}, {"save.tar.lz4", true}, {"save.tar.zst", true},
		} {
			request := model.FileRequest{Operation: operation, Path: item.name, Destination: item.name}

			expanded := item.expanded
			if operation == model.FileOperationExtract || operation == model.FileOperationExtractStart {
				expanded = expanded && item.name != "save.zip" && item.name != "save.tgz"
			} else if item.name != "save.zip" && item.name != "save.tgz" {
				expanded = false
			}

			required := RequiredCapabilities(Command{Name: ServerFiles, Files: &request})
			if slices.Contains(required, CapabilityArchiveFormats) != expanded {
				t.Fatal(operation, item.name, required)
			}

			if expanded {
				old := slices.DeleteFunc(SupportedCapabilities(), func(capability string) bool { return capability == CapabilityArchiveFormats })
				if err := Require(old, required...); err == nil {
					t.Fatal("older Spark accepted new format")
				}
			}
		}
	}

	for _, operation := range []model.FileOperation{model.FileOperationCompressStatus, model.FileOperationCompressCancel, model.FileOperationExtractStatus, model.FileOperationExtractResolve, model.FileOperationExtractCancel} {
		required := RequiredCapabilities(Command{Name: ServerFiles, Files: &model.FileRequest{Operation: operation, Path: "save.tar.xz", Destination: "save.zip"}})
		if slices.Contains(required, CapabilityArchiveFormats) {
			t.Fatal("format gate blocks control of an existing session", operation)
		}
	}
}
