package fileops

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"campfire/internal/shared/model"
)

type destinationFixture struct {
	manager    *Manager
	deployment model.Deployment
	volume     string
	request    model.FileRequest
	source     []byte
}

func newDestinationFixture(t *testing.T) destinationFixture {
	t.Helper()

	volume := t.TempDir()
	if err := os.Mkdir(filepath.Join(volume, "dest"), 0700); err != nil {
		t.Fatal(err)
	}

	var buffer bytes.Buffer

	archive := zip.NewWriter(&buffer)

	member, err := archive.Create("a.txt")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := member.Write([]byte("archive replacement")); err != nil {
		t.Fatal(err)
	}

	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}

	source := append([]byte(nil), buffer.Bytes()...)
	if err := os.WriteFile(filepath.Join(volume, "source.zip"), source, 0600); err != nil {
		t.Fatal(err)
	}

	m := New(Host{FileOwner: func(*os.File, string) error { return nil }})
	t.Cleanup(m.Close)

	return destinationFixture{manager: m, deployment: model.Deployment{ServerID: "game", Configuration: model.RuntimeConfiguration{DiskMB: 1024}}, volume: volume,
		request: model.FileRequest{ServerID: "game", Operation: model.FileOperationExtractStart, Path: "source.zip", Destination: "dest", ExtractionOwner: "owner", Extraction: &model.ExtractionRequest{ID: "12345678901234567890123456789012"}}, source: source}
}

func (f destinationFixture) call(t *testing.T, operation model.FileOperation, choice *model.ExtractionRequest) *model.ExtractionStatus {
	t.Helper()

	q := f.request

	q.Operation = operation
	if choice != nil {
		q.Extraction = choice
	}

	result, err := f.manager.Run(t.Context(), f.deployment, f.volume, q)
	if err != nil {
		t.Fatal(err)
	}

	return result.Extraction
}

func (f destinationFixture) wait(t *testing.T) *model.ExtractionStatus {
	t.Helper()

	for until := time.Now().Add(2 * time.Second); time.Now().Before(until); time.Sleep(time.Millisecond) {
		status := f.call(t, model.FileOperationExtractStatus, nil)
		if status.State != "running" {
			return status
		}
	}

	t.Fatal("extraction did not settle")

	return nil
}

func (f destinationFixture) check(t *testing.T, name string, want []byte) {
	t.Helper()

	got, err := os.ReadFile(filepath.Join(f.volume, name))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s got %q (%v), want %q", name, got, err, want)
	}
}

func TestSourceHardlinksAreProtectedAndMayBeRenamed(t *testing.T) {
	for _, renameSource := range []bool{false, true} {
		t.Run(map[bool]string{false: "source present", true: "source renamed"}[renameSource], func(t *testing.T) {
			f := newDestinationFixture(t)
			if err := os.Link(filepath.Join(f.volume, "source.zip"), filepath.Join(f.volume, "dest/a.txt")); err != nil {
				t.Fatal(err)
			}

			f.call(t, model.FileOperationExtractStart, nil)

			status := f.wait(t)
			if status.Conflict == nil || !status.Conflict.Protected {
				t.Fatal("source hardlink was not protected", status)
			}

			if renameSource {
				if err := os.Rename(filepath.Join(f.volume, "source.zip"), filepath.Join(f.volume, "moved.zip")); err != nil {
					t.Fatal(err)
				}
			}

			q := f.request
			q.Operation = model.FileOperationExtractResolve
			q.Extraction = &model.ExtractionRequest{ID: status.ID, ConflictID: status.Conflict.ID, Action: "overwrite"}

			if renameSource {
				// Renaming another hardlink changes ctime: the original token must
				// refresh first, while the source identity remains protected.
				refreshed := f.call(t, model.FileOperationExtractResolve, q.Extraction)
				if refreshed.Conflict == nil || !refreshed.Conflict.Changed || !refreshed.Conflict.Protected {
					t.Fatal("renamed source lost its protected identity", refreshed)
				}

				q.Extraction.ConflictID = refreshed.Conflict.ID
			}

			var failure *model.FileError
			if _, err := f.manager.Run(t.Context(), f.deployment, f.volume, q); !errors.As(err, &failure) || failure.Code != model.FileConflict {
				t.Fatal("source overwrite accepted", err)
			}

			f.check(t, "dest/a.txt", f.source)

			q.Extraction.Action, q.Extraction.Name = "rename", "renamed.txt"
			f.call(t, model.FileOperationExtractResolve, q.Extraction)

			if status := f.wait(t); status.State != "completed" {
				t.Fatal(status)
			}

			f.check(t, "dest/a.txt", f.source)
			f.check(t, "dest/renamed.txt", []byte("archive replacement"))
		})
	}
}

func TestAllocationReconciliationPrecedesFinalDestinationCheck(t *testing.T) {
	for _, hardlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "new edit", true: "new source hardlink"}[hardlink], func(t *testing.T) {
			f := newDestinationFixture(t)

			target := filepath.Join(f.volume, "dest/a.txt")
			if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}

			f.call(t, model.FileOperationExtractStart, nil)
			status := f.wait(t)
			s := f.manager.operations[f.request.Extraction.ID]
			s.fsMu.Lock()
			s.allocation.refreshed = time.Time{}
			s.allocation.usage = func(ctx context.Context, volume string) (int64, error) {
				if hardlink {
					if err := os.Remove(target); err != nil {
						return 0, err
					}

					if err := os.Link(filepath.Join(volume, "source.zip"), target); err != nil {
						return 0, err
					}
				} else if err := os.WriteFile(target, []byte("newer edit"), 0600); err != nil {
					return 0, err
				}

				return FileUsage(ctx, volume)
			}
			s.fsMu.Unlock()
			f.call(t, model.FileOperationExtractResolve, &model.ExtractionRequest{ID: status.ID, ConflictID: status.Conflict.ID, Action: "overwrite"})

			status = f.wait(t)
			if status.State != "waiting" || status.Conflict == nil || !status.Conflict.Changed || status.Conflict.Protected != hardlink {
				t.Fatal("destination change did not require a fresh answer", status)
			}

			if hardlink {
				f.check(t, "dest/a.txt", f.source)
			} else {
				f.check(t, "dest/a.txt", []byte("newer edit"))
			}

			f.check(t, "source.zip", f.source)
			f.call(t, model.FileOperationExtractResolve, &model.ExtractionRequest{ID: status.ID, ConflictID: status.Conflict.ID, Action: "skip"})

			if status := f.wait(t); status.State != "completed" {
				t.Fatal(status)
			}
		})
	}
}

func TestFreshMemberChangedDuringAllocationKeepsIncomingContents(t *testing.T) {
	f := newDestinationFixture(t)

	ctx, cancel := context.WithCancel(f.manager.ctx)
	defer cancel()

	info, err := os.Stat(filepath.Join(f.volume, "source.zip"))
	if err != nil {
		t.Fatal(err)
	}

	s := &extractionSession{id: f.request.Extraction.ID, owner: f.request.ExtractionOwner, serverID: f.request.ServerID, source: "source.zip", destination: "dest", volume: f.volume, state: "waiting", expires: time.Now().Add(extractionConflictWait), scanDone: true, ctx: ctx, cancel: cancel, sourceInfo: info, answers: make(map[string]model.ExtractionRequest),
		items: []*extractionItem{{entry: model.ExtractionEntry{Path: "a.txt", Kind: model.FileKindFile, Size: 19}, target: "dest/a.txt", status: "pending", parent: -1, mode: 0600}}}
	f.manager.operations[s.id] = s
	s.allocation = &archiveAllocation{limit: 1 << 30, usage: func(ctx context.Context, volume string) (int64, error) {
		if err := os.WriteFile(filepath.Join(volume, "dest/a.txt"), []byte("new destination"), 0600); err != nil {
			return 0, err
		}

		return FileUsage(ctx, volume)
	}}

	root, err := os.OpenRoot(f.volume)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = root.Close() }()

	s.fsMu.Lock()
	err = f.manager.writeExtractionItem(root, f.deployment, s, 0, strings.NewReader("archive replacement"), "absent", "extracted")
	s.fsMu.Unlock()

	if err != nil {
		t.Fatal(err)
	}

	status := f.call(t, model.FileOperationExtractStatus, nil)
	if status.Conflict == nil || !status.Conflict.Changed {
		t.Fatal("new arrival was not presented as a conflict", status)
	}

	f.check(t, "dest/a.txt", []byte("new destination"))
	f.call(t, model.FileOperationExtractResolve, &model.ExtractionRequest{ID: status.ID, ConflictID: status.Conflict.ID, Action: "rename", Name: "renamed.txt"})

	if status := f.wait(t); status.State != "completed" {
		t.Fatal(status)
	}

	f.check(t, "dest/renamed.txt", []byte("archive replacement"))
}
