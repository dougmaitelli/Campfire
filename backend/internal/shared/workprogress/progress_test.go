package workprogress

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return min(3, len(p)), io.ErrClosedPipe }

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) { return copy(p, "abc"), io.ErrUnexpectedEOF }

func TestMeasurementsCountActualIOAndNeverClaimCommit(t *testing.T) {
	var frames []Snapshot

	ctx := WithReporter(t.Context(), func(s Snapshot) { frames = append(frames, s) })
	m := New(ctx, Uploading, 6)

	n, err := io.Copy(m.Writer(failingWriter{}), m.Reader(bytes.NewBufferString("abcdef")))
	if n != 3 || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(n, err)
	}

	m.Flush()

	last := frames[len(frames)-1]
	if last.InputBytes != 6 || last.OutputBytes != 3 || last.InputTotal != 6 || last.Phase != Uploading || last.UpdatedAt.IsZero() {
		t.Fatal(last)
	}
	// The input boundary is emitted before acknowledgement/Flush, even for small
	// streams. Neither an input boundary nor a flush changes the phase to success.
	if len(frames) < 3 || frames[1].InputBytes != 6 {
		t.Fatal(frames)
	}

	if frames[0].InputBytes != 0 {
		t.Fatal("observer snapshots mutated")
	}
}

func TestUnknownAndChangingTotals(t *testing.T) {
	for _, total := range []int64{0, 2} {
		var last Snapshot

		m := New(WithReporter(t.Context(), func(s Snapshot) { last = s }), Validating, total)

		n, err := io.Copy(io.Discard, m.Reader(failingReader{}))
		if n != 3 || !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatal(n, err)
		}

		m.Flush()

		if last.InputBytes != 3 || last.InputTotal != 0 {
			t.Fatal(last)
		}
	}
}

func TestReadersCannotBypassCountingAndObserversAreSerialized(t *testing.T) {
	var last Snapshot

	m := New(WithReporter(t.Context(), func(s Snapshot) { last = s }), Archiving, 0)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				// bytes.Reader has WriterTo; wrapping must still measure every byte.
				if _, err := io.Copy(io.Discard, m.Reader(bytes.NewReader([]byte("data")))); err != nil {
					t.Error(err)
				}

				m.Entry()
			}
		})
	}

	wg.Wait()
	m.Flush()

	if last.InputBytes != 3200 || last.Entries != 800 {
		t.Fatal(last)
	}
}

func TestOutputAndRandomAccessMeasurements(t *testing.T) {
	var last Snapshot

	m := New(WithReporter(t.Context(), func(s Snapshot) { last = s }), Extracting, 0)
	r := m.ReaderAt(bytes.NewReader([]byte("abcdef")))

	var data [4]byte
	for range 2 {
		if _, err := r.ReadAt(data[:], 0); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := io.Copy(io.Discard, m.OutputReader(bytes.NewReader(data[:]))); err != nil {
		t.Fatal(err)
	}

	m.Flush()

	if last.InputBytes != 8 || last.OutputBytes != 4 || last.InputTotal != 0 {
		t.Fatal(last)
	}
	// Observers are optional, and a fresh phase cannot inherit old counters.
	New(context.Background(), Preparing, 0).Flush()
	m = New(WithReporter(t.Context(), func(s Snapshot) { last = s }), Committing, 0)
	m.Flush()

	if last.InputBytes != 0 || last.OutputBytes != 0 || last.Entries != 0 {
		t.Fatal(last)
	}
}
