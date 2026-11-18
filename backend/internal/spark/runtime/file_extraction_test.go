package runtime

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

type extractionFixture struct {
	m      *Manager
	d      model.Deployment
	volume string
	q      model.FileRequest
}

func extractionZIP(t *testing.T, entries map[string]string) []byte {
	t.Helper()

	var buffer bytes.Buffer

	writer := zip.NewWriter(&buffer)

	keys := make([]string, 0, len(entries))
	for name := range entries {
		keys = append(keys, name)
	}

	sort.Strings(keys)

	for _, name := range keys {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := entry.Write([]byte(entries[name])); err != nil {
			t.Fatal(err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return buffer.Bytes()
}
func newExtractionFixture(t *testing.T, entries map[string]string) extractionFixture {
	t.Helper()
	m, d := fileTestManager(t)
	f := extractionFixture{m: m, d: d, volume: filepath.Join(m.root(d.ServerID), "data"), q: model.FileRequest{ServerID: d.ServerID, Operation: model.FileOperationExtractStart, Path: "input.zip", Destination: "existing", ExtractionOwner: "user:1", Extraction: &model.ExtractionRequest{ID: "12345678901234567890123456789012"}}}
	f.write(t, "input.zip", string(extractionZIP(t, entries)))

	if err := os.Mkdir(filepath.Join(f.volume, "existing"), 0755); err != nil {
		t.Fatal(err)
	}

	return f
}
func (f extractionFixture) write(t *testing.T, name, value string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(filepath.Join(f.volume, name)), 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(f.volume, name), []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}
func (f extractionFixture) read(t *testing.T, name, want string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(f.volume, name))
	if err != nil || string(data) != want {
		t.Fatalf("%s got %q, %v; want %q", name, data, err, want)
	}
}
func (f extractionFixture) absent(t *testing.T, name string) {
	t.Helper()

	if _, err := os.Lstat(filepath.Join(f.volume, name)); !os.IsNotExist(err) {
		t.Fatalf("%s unexpectedly exists: %v", name, err)
	}
}
func (f extractionFixture) call(t *testing.T, operation model.FileOperation, choice *model.ExtractionRequest) *model.ExtractionStatus {
	t.Helper()

	q := f.q

	q.Operation = operation
	if choice != nil {
		q.Extraction = choice
	}

	result, err := f.m.Files(context.Background(), f.d, q)
	if err != nil {
		t.Fatal(err)
	}

	if result.Extraction == nil {
		t.Fatal("missing operation status")
	}

	return result.Extraction
}
func (f extractionFixture) wait(t *testing.T, state model.ExtractionState) *model.ExtractionStatus {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		value := f.call(t, model.FileOperationExtractStatus, nil)
		if value.State == state {
			return value
		}

		if value.State == "failed" {
			t.Fatalf("unexpected failure: %+v", value)
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatalf("operation never reached %s", state)

	return nil
}
func (f extractionFixture) answer(t *testing.T, status *model.ExtractionStatus, action model.ExtractionAction, name string) *model.ExtractionStatus {
	t.Helper()

	if status.Conflict == nil {
		t.Fatal("missing conflict")
	}

	return f.call(t, model.FileOperationExtractResolve, &model.ExtractionRequest{ID: status.ID, ConflictID: status.Conflict.ID, Action: action, Name: name})
}
func (f extractionFixture) noStages(t *testing.T) {
	t.Helper()

	entries, err := os.ReadDir(f.volume)
	if err != nil {
		t.Fatal(err)
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".campfire-") {
			t.Fatal("extraction created in-volume staging", entry.Name())
		}
	}
}

func TestExtractionPublishesUnrelatedEntriesBeforeConflictAnswers(t *testing.T) {
	f := newExtractionFixture(t, map[string]string{"a.txt": "new a", "nested/b.txt": "new b", "z.txt": "new z", ".campfire-extract-user.txt": "normal user file"})
	f.write(t, "existing/a.txt", "old a")
	f.write(t, "existing/nested/b.txt", "old b")
	f.call(t, model.FileOperationExtractStart, nil)
	status := f.wait(t, "waiting")
	f.read(t, "existing/z.txt", "new z")
	f.read(t, "existing/.campfire-extract-user.txt", "normal user file")
	f.read(t, "existing/a.txt", "old a")
	f.noStages(t)

	if status.PendingConflicts != 2 {
		t.Fatalf("expected two file conflicts: %+v", status)
	}

	f.answer(t, status, "overwrite", "")
	status = f.wait(t, "waiting")
	f.read(t, "existing/a.txt", "new a")
	f.answer(t, status, "skip", "")
	done := f.wait(t, "completed")
	f.read(t, "existing/nested/b.txt", "old b")

	if done.RemainingEntries != 0 {
		t.Fatal(done)
	}
}
func TestExtractionBlockingDirectoryOnlyHoldsItsSubtree(t *testing.T) {
	for _, action := range []model.ExtractionAction{model.ExtractionActionRename, model.ExtractionActionSkip} {
		t.Run(string(action), func(t *testing.T) {
			f := newExtractionFixture(t, map[string]string{"config/deep/a.txt": "a", "z.txt": "z"})
			f.write(t, "existing/config", "blocking file")
			f.call(t, model.FileOperationExtractStart, nil)
			status := f.wait(t, "waiting")
			f.read(t, "existing/z.txt", "z")

			if status.Conflict == nil || status.Conflict.Path != "config" {
				t.Fatal(status)
			}

			f.answer(t, status, action, "config copy")
			f.wait(t, "completed")
			f.read(t, "existing/config", "blocking file")

			if action == "rename" {
				f.read(t, "existing/config copy/deep/a.txt", "a")
			} else {
				f.absent(t, "existing/config copy")
			}
		})
	}
}
func TestExtractionRejectsStaleAnswersAndUsesOriginalIncomingContents(t *testing.T) {
	f := newExtractionFixture(t, map[string]string{"a.txt": "incoming"})
	f.write(t, "existing/a.txt", "old")
	f.call(t, model.FileOperationExtractStart, nil)
	first := f.wait(t, "waiting")
	f.write(t, "existing/a.txt", "changed by game")

	next := f.answer(t, first, "overwrite", "")
	if next.Conflict == nil || next.Conflict.ID == first.Conflict.ID || !next.Conflict.Changed {
		t.Fatal("stale decision was not refreshed", next)
	}

	f.read(t, "existing/a.txt", "changed by game")
	f.write(t, "input.zip", "archive changed after entry was read")
	f.answer(t, next, "overwrite", "")
	f.wait(t, "completed")
	f.read(t, "existing/a.txt", "incoming")
	// A lost response may be retried but must never repeat publication.
	f.write(t, "existing/a.txt", "later edit")
	f.answer(t, next, "overwrite", "")
	f.read(t, "existing/a.txt", "later edit")
}
func TestExtractionCancellationKeepsCompletedFilesAndCleansExternalTemporaryFiles(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	f := newExtractionFixture(t, map[string]string{"a.txt": "new", "z.txt": "saved"})
	f.write(t, "existing/a.txt", "old")
	f.call(t, model.FileOperationExtractStart, nil)
	f.wait(t, "waiting")

	files, err := os.ReadDir(temporary)
	if err != nil || len(files) == 0 {
		t.Fatal("expected external waiting content", err)
	}

	status := f.call(t, model.FileOperationExtractCancel, nil)
	if status.State != "cancelled" {
		t.Fatal(status)
	}

	f.read(t, "existing/z.txt", "saved")
	f.read(t, "existing/a.txt", "old")
	f.noStages(t)

	files, err = os.ReadDir(temporary)
	if err != nil || len(files) != 0 {
		t.Fatal("waiting content leaked", files, err)
	}
}
func TestExtractionOwnerAndRequestAreBoundToOperation(t *testing.T) {
	f := newExtractionFixture(t, map[string]string{"a.txt": "new"})
	f.write(t, "existing/a.txt", "old")
	f.call(t, model.FileOperationExtractStart, nil)
	f.wait(t, "waiting")

	for _, change := range []func(*model.FileRequest){func(q *model.FileRequest) { q.ExtractionOwner = "other" }, func(q *model.FileRequest) { q.Path = "other.zip" }, func(q *model.FileRequest) { q.Destination = "." }} {
		q := f.q
		q.Operation = model.FileOperationExtractStatus
		change(&q)

		if _, err := f.m.Files(t.Context(), f.d, q); err == nil {
			t.Fatal("foreign operation accepted")
		}
	}
}
func TestExtractionFileDirectoryMismatchAndSourceProtection(t *testing.T) {
	f := newExtractionFixture(t, map[string]string{"a": "incoming"})
	if err := os.Mkdir(filepath.Join(f.volume, "existing/a"), 0700); err != nil {
		t.Fatal(err)
	}

	f.call(t, model.FileOperationExtractStart, nil)
	status := f.wait(t, "waiting")
	q := f.q
	q.Operation = model.FileOperationExtractResolve

	q.Extraction = &model.ExtractionRequest{ID: status.ID, ConflictID: status.Conflict.ID, Action: "overwrite"}
	if _, err := f.m.Files(t.Context(), f.d, q); err == nil {
		t.Fatal("overwrote directory")
	}

	f.answer(t, status, "rename", "a copy")
	f.wait(t, "completed")
	f.read(t, "existing/a copy", "incoming")
	protected := newExtractionFixture(t, map[string]string{"input.zip": "not an archive"})
	protected.q.Destination = "."
	protected.call(t, model.FileOperationExtractStart, nil)

	status = protected.wait(t, "waiting")
	if status.Conflict == nil || !status.Conflict.Protected {
		t.Fatal("source not protected")
	}

	q = protected.q
	q.Operation = model.FileOperationExtractResolve

	q.Extraction = &model.ExtractionRequest{ID: status.ID, ConflictID: status.Conflict.ID, Action: "overwrite"}
	if _, err := protected.m.Files(t.Context(), protected.d, q); err == nil {
		t.Fatal("replaced source archive")
	}

	protected.answer(t, status, "skip", "")
	protected.wait(t, "completed")
}
func TestExtractionDuplicateAndBoundsStopWithoutRollingBackCompletedEntries(t *testing.T) {
	for _, test := range []struct {
		name    string
		entries map[string]string
		message string
	}{{"duplicate", map[string]string{"./a.txt": "first", "a.txt": "duplicate"}, "duplicate"}, {"limit", func() map[string]string {
		entries := map[string]string{}
		for i := 0; i < 1001; i++ {
			entries[fmt.Sprintf("item-%04d", i)] = "x"
		}

		return entries
	}(), "1,000"}} {
		t.Run(test.name, func(t *testing.T) {
			f := newExtractionFixture(t, test.entries)
			f.call(t, model.FileOperationExtractStart, nil)

			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				status := f.call(t, model.FileOperationExtractStatus, nil)
				if status.State == "failed" {
					if !strings.Contains(status.Message, test.message) {
						t.Fatal(status.Message)
					}

					if len(status.Outcomes) == 0 {
						t.Fatal("lost partial results")
					}

					f.noStages(t)

					return
				}

				time.Sleep(time.Millisecond)
			}

			t.Fatal("expected partial failure")
		})
	}
}
func TestExtractionChangedDirectoryAndRemovedFileRequireFreshDecisions(t *testing.T) {
	t.Run("directory", func(t *testing.T) {
		f := newExtractionFixture(t, map[string]string{"config/a": "incoming"})
		f.write(t, "existing/config", "old file")
		f.call(t, model.FileOperationExtractStart, nil)

		first := f.wait(t, "waiting")
		if err := os.Remove(filepath.Join(f.volume, "existing/config")); err != nil {
			t.Fatal(err)
		}

		if err := os.Mkdir(filepath.Join(f.volume, "existing/config"), 0700); err != nil {
			t.Fatal(err)
		}

		next := f.answer(t, first, "rename", "other")
		if next.Conflict == nil || !next.Conflict.Changed || next.Conflict.Existing.Kind != model.FileKindDirectory {
			t.Fatal(next)
		}

		f.answer(t, next, "merge", "")
		f.wait(t, "completed")
		f.read(t, "existing/config/a", "incoming")
	})
	t.Run("removed file", func(t *testing.T) {
		f := newExtractionFixture(t, map[string]string{"a": "incoming"})
		f.write(t, "existing/a", "old")
		f.call(t, model.FileOperationExtractStart, nil)

		first := f.wait(t, "waiting")
		if err := os.Remove(filepath.Join(f.volume, "existing/a")); err != nil {
			t.Fatal(err)
		}

		next := f.answer(t, first, "overwrite", "")
		if next.Conflict == nil || !next.Conflict.Changed || next.Conflict.Existing != nil {
			t.Fatal(next)
		}

		f.answer(t, next, "extract", "")
		f.wait(t, "completed")
		f.read(t, "existing/a", "incoming")
	})
}

func TestFreshExtractionRetainsLargerEntryBudget(t *testing.T) {
	entries := make(map[string]string)
	for i := range 1001 {
		entries[fmt.Sprintf("file-%04d", i)] = "x"
	}

	f := newExtractionFixture(t, entries)
	q := f.q
	q.Operation = model.FileOperationExtract
	q.Destination = "new folder"

	q.Extraction = nil
	if _, err := f.m.Files(t.Context(), f.d, q); err != nil {
		t.Fatal(err)
	}

	f.read(t, "new folder/file-0000", "x")
	f.read(t, "new folder/file-1000", "x")
	f.noStages(t)
}

func TestExtractionIntoCurrentOrNewFolderNeedsNoDestinationQuestion(t *testing.T) {
	for _, destination := range []string{".", "new folder"} {
		t.Run(destination, func(t *testing.T) {
			f := newExtractionFixture(t, map[string]string{"a.txt": "a"})
			f.q.Destination = destination
			f.call(t, model.FileOperationExtractStart, nil)

			status := f.wait(t, "completed")
			if status.Conflict != nil {
				t.Fatal(status)
			}

			f.read(t, filepath.Join(destination, "a.txt"), "a")
			f.noStages(t)
		})
	}
}
