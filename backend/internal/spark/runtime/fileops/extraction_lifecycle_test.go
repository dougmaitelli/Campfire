package fileops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
)

func TestSnapshotDoesNotPublishInternalWorkPhases(t *testing.T) {
	s := &extractionSession{state: model.ExtractionRunning, items: []*extractionItem{
		{entry: model.ExtractionEntry{Path: "writing"}, status: extractionWriting},
		{entry: model.ExtractionEntry{Path: "resolving"}, status: extractionResolving},
	}}

	snapshot := s.snapshot()
	if snapshot.RemainingEntries != 2 {
		t.Fatal(snapshot)
	}

	for _, item := range snapshot.Outcomes {
		if item.Status != model.ExtractionOutcomePending {
			t.Fatal("internal phase escaped into protocol", item.Status)
		}
	}
}

func TestConflictDeadlineExpiresWithoutRequestsAndPollingDoesNotExtendIt(t *testing.T) {
	m := New(Host{})
	defer m.Close()

	ctx, cancel := context.WithCancel(m.ctx)

	temporary := filepath.Join(t.TempDir(), "pending-content")
	if err := os.Mkdir(temporary, 0700); err != nil {
		t.Fatal(err)
	}

	s := &extractionSession{id: "12345678901234567890123456789012", owner: "owner", serverID: "server", source: "archive.zip", destination: ".", state: "waiting", scanDone: true, ctx: ctx, cancel: cancel, expires: time.Now().Add(80 * time.Millisecond), temporary: temporary, items: []*extractionItem{{entry: model.ExtractionEntry{Path: "a"}, status: "pending", conflict: &model.ExtractionConflict{ID: "conflict"}}}}
	m.operations[s.id] = s
	m.armExtraction(s)

	s.mu.Lock()
	initial := s.expires
	s.mu.Unlock()

	for range 3 {
		result, err := m.Run(t.Context(), model.Deployment{}, "", model.FileRequest{ServerID: s.serverID, Operation: model.FileOperationExtractStatus, Path: s.source, Destination: s.destination, ExtractionOwner: s.owner, Extraction: &model.ExtractionRequest{ID: s.id}})
		if err != nil {
			t.Fatal(err)
		}

		if !result.Extraction.ExpiresAt.Equal(initial) {
			t.Fatal("status renewed the deadline")
		}

		time.Sleep(5 * time.Millisecond)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		state := s.state
		s.mu.Unlock()

		if state == "expired" {
			if _, err := os.Stat(temporary); !os.IsNotExist(err) {
				t.Fatal("expired staging was not cleaned", err)
			}

			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("idle conflict did not expire autonomously")
}

func TestQueuedTimerDoesNotExpireRenewedWait(t *testing.T) {
	for _, invalidate := range []bool{false, true} {
		t.Run(fmt.Sprint(invalidate), func(t *testing.T) {
			m := New(Host{})
			defer m.Close()

			ctx, cancel := context.WithCancel(m.ctx)
			s := &extractionSession{id: "renewed", state: "waiting", ctx: ctx, cancel: cancel, scanDone: true, expires: time.Now().Add(10 * time.Millisecond)}
			m.operations[s.id] = s
			m.armExtraction(s)
			s.mu.Lock()
			// Let the previous deadline pass while its callback waits for the lock.
			time.Sleep(30 * time.Millisecond)

			if invalidate {
				s.stopTimer()
			}

			s.expires = time.Now().Add(time.Hour)
			s.mu.Unlock()
			time.Sleep(30 * time.Millisecond)
			s.mu.Lock()
			defer s.mu.Unlock()

			if s.state != "waiting" {
				t.Fatal("old timer expired the renewed deadline", s.state)
			}
		})
	}
}

func TestRequestDetectedExpiryRetainsResultsWithoutRenewingOnPoll(t *testing.T) {
	m := New(Host{})
	defer m.Close()

	ctx, cancel := context.WithCancel(m.ctx)
	s := &extractionSession{id: "12345678901234567890123456789012", owner: "owner", serverID: "server", source: "archive.zip", destination: ".", state: "waiting", scanDone: true, ctx: ctx, cancel: cancel, expires: time.Now().Add(time.Hour)}
	m.operations[s.id] = s
	m.armExtraction(s)
	s.mu.Lock()
	s.expires = time.Now().Add(-time.Second)
	s.mu.Unlock()
	q := model.FileRequest{ServerID: s.serverID, Operation: model.FileOperationExtractStatus, Path: s.source, Destination: s.destination, ExtractionOwner: s.owner, Extraction: &model.ExtractionRequest{ID: s.id}}

	before := time.Now()

	result, err := m.Run(t.Context(), model.Deployment{}, "", q)
	if err != nil || result.Extraction.State != "expired" {
		t.Fatal("request did not detect expiry", result, err)
	}

	after := time.Now()

	s.mu.Lock()
	retention := s.expires
	s.mu.Unlock()

	if retention.Before(before.Add(limits.ArchiveResultRetention)) || retention.After(after.Add(limits.ArchiveResultRetention)) {
		t.Fatal("expired result was not retained for recovery")
	}

	if _, err := m.Run(t.Context(), model.Deployment{}, "", q); err != nil {
		t.Fatal(err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.expires.Equal(retention) {
		t.Fatal("polling renewed result retention")
	}
}

func TestCloseCancelsPendingOperationsAndRemovesExternalContent(t *testing.T) {
	m := New(Host{})
	ctx, cancel := context.WithCancel(m.ctx)

	temporary := filepath.Join(t.TempDir(), "waiting")
	if err := os.Mkdir(temporary, 0700); err != nil {
		t.Fatal(err)
	}

	s := &extractionSession{id: "id", state: "waiting", ctx: ctx, cancel: cancel, temporary: temporary, scanDone: true}
	m.operations[s.id] = s
	m.Close()

	if _, err := os.Stat(temporary); !os.IsNotExist(err) {
		t.Fatal("temporary content survived close", err)
	}
}
