package fileops

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/shared/model"
	"campfire/internal/shared/workprogress"
	"campfire/internal/spark/runtime/fileops/archive"

	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

var tarFormats = []string{"tar", "tar.gz", "tar.xz", "tar.lz", "tar.bz2", "tar.lz4", "tar.zst"}

func tarFixture(t *testing.T, variant, format string) []byte {
	t.Helper()

	suffix := format
	if format == "tar.gz" || format == "tar.lz4" || format == "tar.zst" {
		suffix = "tar"
	}

	data, err := os.ReadFile(filepath.Join("archive", "testdata", variant+"."+suffix))
	if err != nil {
		t.Fatal(err)
	}

	var (
		encoded bytes.Buffer
		writer  io.WriteCloser
	)

	switch format {
	case "tar.gz":
		writer = gzip.NewWriter(&encoded)
	case "tar.lz4":
		writer = lz4.NewWriter(&encoded)
	case "tar.zst":
		writer, err = zstd.NewWriter(&encoded, zstd.WithEncoderConcurrency(1))
		if err != nil {
			t.Fatal(err)
		}
	default:
		return data
	}

	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	return encoded.Bytes()
}

func archiveTestManager(t *testing.T) *Manager {
	t.Helper()

	m := New(Host{FileOwner: func(*os.File, string) error { return nil }})
	t.Cleanup(m.Close)

	return m
}

func TestTarFormatsShareExtractionPolicy(t *testing.T) {
	for _, format := range tarFormats {
		for _, variant := range []string{"valid", "duplicate", "traversal", "link", "special", "type-conflict", "oversized"} {
			t.Run(format+"/"+variant, func(t *testing.T) {
				volume := t.TempDir()
				if err := os.WriteFile(filepath.Join(volume, "input."+format), tarFixture(t, variant, format), 0600); err != nil {
					t.Fatal(err)
				}

				m := archiveTestManager(t)
				q := model.FileRequest{ServerID: "server", Operation: model.FileOperationExtract, Path: "input." + format, Destination: "output"}

				var progress workprogress.Snapshot

				ctx := workprogress.WithReporter(t.Context(), func(s workprogress.Snapshot) { progress = s })

				_, err := m.Files(ctx, model.Deployment{ServerID: "server"}, volume, &State{}, q)
				if variant != "valid" {
					if err == nil {
						t.Fatal("unsafe archive accepted")
					}

					if variant == "duplicate" {
						data, _ := os.ReadFile(filepath.Join(volume, "output", "save.txt"))
						if string(data) != "first" {
							t.Fatalf("first duplicate was replaced: %q", data)
						}
					}

					if _, err := os.Stat(filepath.Join(volume, "outside")); !os.IsNotExist(err) {
						t.Fatal("traversal escaped", err)
					}

					return
				}

				if err != nil {
					t.Fatal(err)
				}

				data, err := os.ReadFile(filepath.Join(volume, "output/world/save.txt"))
				if err != nil || string(data) != "saved world\n" {
					t.Fatal(string(data), err)
				}

				info, err := os.Stat(filepath.Join(volume, "output/world/save.txt"))
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal(info, err)
				}

				if progress.InputTotal == 0 || progress.InputBytes != progress.InputTotal || progress.OutputBytes != int64(len(data)) || progress.Entries != 2 {
					t.Fatal(progress)
				}
			})
		}
	}
}

func TestTarFormatsInteractiveConflictsAndFreshExtraction(t *testing.T) {
	cases := []struct {
		name          string
		fresh, cancel bool
		action        model.ExtractionAction
		outcome       model.ExtractionOutcomeStatus
	}{
		{name: "fresh", fresh: true},
		{name: "cancel", cancel: true, outcome: model.ExtractionOutcomeCancelled},
		{name: "overwrite", action: model.ExtractionActionOverwrite, outcome: model.ExtractionOutcomeOverwritten},
		{name: "rename", action: model.ExtractionActionRename, outcome: model.ExtractionOutcomeRenamed},
		{name: "skip", action: model.ExtractionActionSkip, outcome: model.ExtractionOutcomeSkipped},
	}
	for _, format := range tarFormats {
		for _, tc := range cases {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				volume := t.TempDir()
				if err := os.WriteFile(filepath.Join(volume, "input."+format), tarFixture(t, "valid", format), 0600); err != nil {
					t.Fatal(err)
				}

				original := filepath.Join(volume, "output/world/save.txt")
				if !tc.fresh {
					if err := os.MkdirAll(filepath.Dir(original), 0700); err != nil {
						t.Fatal(err)
					}

					if err := os.WriteFile(original, []byte("keep"), 0600); err != nil {
						t.Fatal(err)
					}
				}

				m := archiveTestManager(t)

				const id = "12345678901234567890123456789012"

				q := model.FileRequest{ServerID: "server", Operation: model.FileOperationExtractStart, Path: "input." + format, Destination: "output", ExtractionOwner: "owner", Extraction: &model.ExtractionRequest{ID: id, Fresh: tc.fresh}}

				result, err := m.Run(t.Context(), model.Deployment{}, volume, q)
				if err != nil {
					t.Fatal(err)
				}

				deadline := time.Now().Add(3 * time.Second)
				handled := false

				for !result.Extraction.State.Terminal() {
					if time.Now().After(deadline) {
						t.Fatal("operation did not finish", result.Extraction)
					}

					q.Operation = model.FileOperationExtractStatus

					q.Extraction = &model.ExtractionRequest{ID: id, Fresh: tc.fresh}
					if !handled && result.Extraction.Conflict != nil {
						if tc.fresh {
							t.Fatal("fresh extraction asked about a conflict", result.Extraction)
						}

						data, err := os.ReadFile(original)
						if err != nil || string(data) != "keep" {
							t.Fatal("conflict replaced existing contents", string(data), err)
						}

						handled = true

						if tc.cancel {
							q.Operation = model.FileOperationExtractCancel
						} else {
							q.Operation = model.FileOperationExtractResolve
							q.Extraction.ConflictID = result.Extraction.Conflict.ID

							q.Extraction.Action = tc.action
							if tc.action == model.ExtractionActionRename {
								q.Extraction.Name = "renamed.txt"
							}
						}
					} else {
						time.Sleep(time.Millisecond)
					}

					result, err = m.Run(t.Context(), model.Deployment{}, volume, q)
					if err != nil {
						t.Fatal(err)
					}
				}

				wantState := model.ExtractionCompleted
				if tc.cancel {
					wantState = model.ExtractionCancelled
				}

				if result.Extraction.State != wantState || (!tc.fresh && !handled) {
					t.Fatal("unexpected terminal result", handled, result.Extraction)
				}

				wantContent := "keep"
				if tc.fresh || tc.action == model.ExtractionActionOverwrite {
					wantContent = "saved world\n"
				}

				data, err := os.ReadFile(original)
				if err != nil || string(data) != wantContent {
					t.Fatal("incorrect destination contents", string(data), err)
				}

				renamed := filepath.Join(volume, "output/world/renamed.txt")
				if tc.action == model.ExtractionActionRename {
					data, err := os.ReadFile(renamed)
					if err != nil || string(data) != "saved world\n" {
						t.Fatal("incorrect renamed contents", string(data), err)
					}
				} else if _, err := os.Stat(renamed); !os.IsNotExist(err) {
					t.Fatal("unexpected rename destination", err)
				}

				if !tc.fresh {
					found := false

					for _, outcome := range result.Extraction.Outcomes {
						if outcome.Path != "world/save.txt" {
							continue
						}

						found = true

						if outcome.Status != tc.outcome {
							t.Fatal("incorrect entry outcome", outcome)
						}

						wantDestination := "output/world/save.txt"
						if tc.action == model.ExtractionActionRename {
							wantDestination = "output/world/renamed.txt"
						}

						if outcome.Destination != wantDestination {
							t.Fatal("incorrect reported destination", outcome)
						}
					}

					if !found {
						t.Fatal("missing entry outcome", result.Extraction)
					}
				}
			})
		}
	}
}

func TestTarFormatCorruptionCancellationAndTrailingMembers(t *testing.T) {
	for _, format := range tarFormats {
		t.Run(format, func(t *testing.T) {
			data := tarFixture(t, "valid", format)
			if format != "tar" {
				for _, damaged := range [][]byte{data[:len(data)/2], append(bytes.Clone(data[:len(data)-1]), data[len(data)-1]^0xff)} {
					if err := readArchiveFixture(t.Context(), t, format, damaged); err == nil {
						t.Fatal("corrupt codec accepted")
					}
				}
			}

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			if err := readArchiveFixture(ctx, t, format, data); !errors.Is(err, context.Canceled) {
				t.Fatal("cancel not respected", err)
			}
			// A second compressed member may contain more bytes of the same tar,
			// but not another independent tar whose files would be silently ignored.
			if err := readArchiveFixture(t.Context(), t, format, append(bytes.Clone(data), data...)); err == nil {
				t.Fatal("concatenated archives silently skipped")
			}
		})
	}
}

func readArchiveFixture(ctx context.Context, t *testing.T, format string, data []byte) error {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "archive")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = file.Close() }()

	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	return ReadArchive(ctx, file, "input."+format, func(entry archive.Entry, reader io.Reader) error {
		_, err := io.CopyN(io.Discard, reader, entry.Size)
		return err
	})
}

func TestZIPCreationRoundTrip(t *testing.T) {
	volume := t.TempDir()
	if err := os.Mkdir(filepath.Join(volume, "world"), 0700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(volume, "world/save.txt"), []byte("saved world\n"), 0600); err != nil {
		t.Fatal(err)
	}

	m := archiveTestManager(t)

	var progress workprogress.Snapshot

	ctx := workprogress.WithReporter(t.Context(), func(s workprogress.Snapshot) {
		if s.Phase == workprogress.Archiving {
			progress = s
		}
	})

	q := model.FileRequest{ServerID: "server", Operation: model.FileOperationCompress, Path: "world", Destination: "world.zip"}
	if _, err := m.Files(ctx, model.Deployment{ServerID: "server"}, volume, &State{}, q); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(volume, "world.zip"))
	if err != nil || progress.InputBytes != 12 || progress.OutputBytes != info.Size() || progress.InputTotal != 0 || progress.Entries != 2 {
		t.Fatal(progress, info, err)
	}

	q.Operation, q.Path, q.Destination = model.FileOperationExtract, "world.zip", "restored"
	if _, err := m.Files(t.Context(), model.Deployment{ServerID: "server"}, volume, &State{}, q); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(volume, "restored/world/save.txt"))
	if err != nil || string(data) != "saved world\n" {
		t.Fatal(string(data), err)
	}
}
