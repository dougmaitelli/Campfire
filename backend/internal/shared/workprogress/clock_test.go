package workprogress

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestThrottleKeepsMonotonicTimeSeparateFromPublicTimestamp(t *testing.T) {
	m := New(context.Background(), Archiving, 0)
	// Round(0) strips monotonic data. Keeping it in last ensures Sub uses
	// elapsed time, not wall-clock adjustments, to throttle reports. Compare
	// representations here: Time.Equal deliberately ignores this distinction.
	if reflect.DeepEqual(m.last, m.last.Round(0)) {
		t.Fatalf("throttle timestamp lost its monotonic clock: last=%v now=%v", m.last, time.Now())
	}

	if m.value.UpdatedAt.Location() != time.UTC || !m.value.UpdatedAt.Equal(m.last) {
		t.Fatal("public timestamp must represent the same instant in UTC")
	}

	if !reflect.DeepEqual(m.value.UpdatedAt, m.value.UpdatedAt.Round(0)) {
		t.Fatal("transport timestamp retained private monotonic data")
	}
}
