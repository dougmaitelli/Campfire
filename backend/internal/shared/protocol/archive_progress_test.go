package protocol

import (
	"campfire/internal/shared/model"
	"slices"
	"testing"
)

func TestObservableArchivesRequireNegotiatedProgressCapability(t *testing.T) {
	for _, operation := range []model.FileOperation{model.FileOperationCompressStart, model.FileOperationCompressStatus, model.FileOperationCompressCancel, model.FileOperationExtractStart, model.FileOperationExtractStatus, model.FileOperationExtractCancel} {
		command := Command{Name: ServerFiles, Files: &model.FileRequest{Operation: operation, Extraction: &model.ExtractionRequest{Fresh: true}}}

		required := RequiredCapabilities(command)
		if !slices.Contains(required, CapabilityArchiveProgress) {
			t.Fatal(operation, required)
		}

		if err := Require([]string{CapabilityFiles, CapabilityFileArchive, CapabilityExtractConflicts}, required...); err == nil {
			t.Fatal("old Spark accepted observable archive protocol", operation)
		}
	}
}
