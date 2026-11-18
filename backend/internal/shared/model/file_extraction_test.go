package model

import (
	"encoding/json"
	"testing"
)

func TestExtractionProtocolValues(t *testing.T) {
	for _, test := range []struct {
		value any
		want  string
	}{
		{ExtractionActionSkip, "skip"}, {ExtractionActionRename, "rename"}, {ExtractionActionOverwrite, "overwrite"}, {ExtractionActionMerge, "merge"}, {ExtractionActionExtract, "extract"},
		{ExtractionRunning, "running"}, {ExtractionWaiting, "waiting"}, {ExtractionCompleted, "completed"}, {ExtractionCancelled, "cancelled"}, {ExtractionExpired, "expired"}, {ExtractionFailed, "failed"},
		{ExtractionOutcomePending, "pending"}, {ExtractionOutcomeExtracted, "extracted"}, {ExtractionOutcomeMerged, "merged"}, {ExtractionOutcomeOverwritten, "overwritten"}, {ExtractionOutcomeRenamed, "renamed"}, {ExtractionOutcomeSkipped, "skipped"}, {ExtractionOutcomeFailed, "failed"}, {ExtractionOutcomeCancelled, "cancelled"},
	} {
		encoded, err := json.Marshal(test.value)
		if err != nil || string(encoded) != `"`+test.want+`"` {
			t.Fatalf("%T wire value: %s, %v", test.value, encoded, err)
		}
	}

	for _, state := range []ExtractionState{ExtractionRunning, ExtractionWaiting, ExtractionCompleted, ExtractionCancelled, ExtractionExpired, ExtractionFailed} {
		want := state != ExtractionRunning && state != ExtractionWaiting
		if state.Terminal() != want {
			t.Fatalf("%s terminal=%v", state, state.Terminal())
		}
	}

	if ExtractionState("unknown").Terminal() {
		t.Fatal("unknown state considered terminal")
	}
}

func TestExtractionStatusOrCancelClassification(t *testing.T) {
	for _, operation := range []FileOperation{FileOperationList, FileOperationRead, FileOperationExtract, FileOperationExtractStart, FileOperationExtractStatus, FileOperationExtractResolve, FileOperationExtractCancel, FileOperationUploadCancel} {
		want := operation == FileOperationExtractStatus || operation == FileOperationExtractCancel
		if got := operation.IsExtractionStatusOrCancel(); got != want {
			t.Errorf("%s: status/cancel=%v, want %v", operation, got, want)
		}
	}
}
