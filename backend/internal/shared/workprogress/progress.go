// Package workprogress measures streaming work without claiming that its owner
// has committed an operation. Counters describe one phase, not overall completion.
package workprogress

import (
	"context"
	"io"
	"sync"
	"time"
)

// Phase identifies the current kind of streaming work, not a terminal state.
type Phase string

// Streaming phases are deliberately separate from operation success or failure.
const (
	Preparing   Phase = "preparing"
	Archiving   Phase = "archiving"
	Extracting  Phase = "extracting"
	Validating  Phase = "validating"
	Uploading   Phase = "uploading"
	Downloading Phase = "downloading"
	Restoring   Phase = "restoring"
	Committing  Phase = "committing"
)

// Snapshot contains measured bytes, never estimated filesystem totals. A zero
// InputTotal means unknown. OutputBytes can differ substantially from InputBytes
// under compression. Reaching a total is not an acknowledgement of publication.
// No paths, credentials, or archive member names are exposed.
type Snapshot struct {
	Phase       Phase     `json:"phase"`
	InputBytes  int64     `json:"input_bytes"`
	OutputBytes int64     `json:"output_bytes"`
	InputTotal  int64     `json:"input_total,omitempty"`
	Entries     int64     `json:"entries"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Reporter receives immutable, bounded snapshots with no resource identities.
type Reporter func(Snapshot)
type reporterKey struct{}

// WithReporter attaches an observer to streaming work, independent of a UI or
// transport. Observers must return promptly and must not call back into a Meter.
func WithReporter(ctx context.Context, report Reporter) context.Context {
	return context.WithValue(ctx, reporterKey{}, report)
}

// Meter serializes observers and throttles notifications, not measurements.
// Flush must be called after streams are closed, including on error. It reports
// the final measured counters but never marks an operation successful.
type Meter struct {
	mu     sync.Mutex
	value  Snapshot
	report Reporter
	last   time.Time
}

// New starts a fresh phase and immediately publishes its zero counters.
func New(ctx context.Context, phase Phase, inputTotal int64) *Meter {
	report, _ := ctx.Value(reporterKey{}).(Reporter)
	m := &Meter{value: Snapshot{Phase: phase, InputTotal: max(0, inputTotal)}, report: report}
	m.Flush()

	return m
}

func (m *Meter) publish(force bool) {
	// Preserve monotonic time for throttling across wall-clock corrections.
	now := time.Now()
	if !force && now.Sub(m.last) < 200*time.Millisecond {
		return
	}

	m.last = now

	m.value.UpdatedAt = now.UTC()
	if m.report != nil {
		m.report(m.value)
	}
}

func (m *Meter) add(input, output, entries int64) {
	if m.report == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	previous := m.value.InputBytes
	m.value.InputBytes += input
	boundary := m.value.InputTotal > 0 && previous < m.value.InputTotal && m.value.InputBytes >= m.value.InputTotal
	m.value.OutputBytes += output
	m.value.Entries += entries
	// An input that grew invalidates its old denominator rather than producing
	// an impossible percentage or silently clamping away actual work.
	if m.value.InputBytes > m.value.InputTotal {
		m.value.InputTotal = 0
	}

	m.publish(boundary)
}

// Entry counts a stream or file member, not a confirmed publication.
func (m *Meter) Entry() { m.add(0, 0, 1) }

// Flush publishes all measurements, without implying validation or commit.
func (m *Meter) Flush() { m.mu.Lock(); defer m.mu.Unlock(); m.publish(true) }

// Reader counts bytes actually returned, including reads that return an error.
// With an observer it hides WriterTo, which could bypass measurement. Without
// one, the original stream and its optimizations are preserved.
func (m *Meter) Reader(r io.Reader) io.Reader {
	if m.report == nil {
		return r
	}

	return &reader{r: r, meter: m}
}

// ReaderAt measures random-access input. Callers must leave InputTotal unknown
// when a decoder can reread bytes; this counter is not unique archive coverage.
func (m *Meter) ReaderAt(r io.ReaderAt) io.ReaderAt {
	if m.report == nil {
		return r
	}

	return &readerAt{r: r, meter: m}
}

type readerAt struct {
	r     io.ReaderAt
	meter *Meter
}

func (r *readerAt) ReadAt(p []byte, offset int64) (int, error) {
	n, err := r.r.ReadAt(p, offset)
	r.meter.add(int64(n), 0, 0)

	return n, err
}

// Writer counts bytes accepted by the destination, including partial writes.
func (m *Meter) Writer(w io.Writer) io.Writer {
	if m.report == nil {
		return w
	}

	return &writer{w: w, meter: m}
}

type reader struct {
	r     io.Reader
	meter *Meter
}

func (r *reader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.meter.add(int64(n), 0, 0)

	return n, err
}

// InputWriter measures payload accepted by an encoder, excluding framing bytes.
func (m *Meter) InputWriter(w io.Writer) io.Writer {
	if m.report == nil {
		return w
	}

	return &writer{w: w, meter: m, input: true}
}

type writer struct {
	w     io.Writer
	meter *Meter
	input bool
}

func (w *writer) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if w.input {
		w.meter.add(int64(n), 0, 0)
	} else {
		w.meter.add(0, int64(n), 0)
	}

	return n, err
}

// OutputReader measures decoded payload delivered to a consumer, separately
// from compressed bytes read by a decoder. Skipped payload is not claimed as
// extracted or published data.
func (m *Meter) OutputReader(r io.Reader) io.Reader {
	if m.report == nil {
		return r
	}

	return &outputReader{r: r, meter: m}
}

type outputReader struct {
	r     io.Reader
	meter *Meter
}

func (r *outputReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.meter.add(0, int64(n), 0)

	return n, err
}
