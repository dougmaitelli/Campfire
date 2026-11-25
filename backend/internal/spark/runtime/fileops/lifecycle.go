// Package fileops implements server-file management, archive extraction and
// short-lived conflict waits.
package fileops

import (
	"context"
	"os"
	"sync"
	"time"

	"campfire/internal/shared/limits"
	"campfire/internal/shared/model"
)

// Host supplies game-file ownership and the disk allocation enforced in software.
type Host struct {
	FileOwner          func(*os.File, string) error
	LogicalDiskLimitMB func(int64) int64
}

// Manager owns file operations and their temporary conflict state.
type Manager struct {
	Host
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	compressions map[string]*compressionSession
	operations   map[string]*extractionSession
	closed       bool
	wg           sync.WaitGroup
}

// State contains upload state protected by the runtime's server-entry lock.
type State struct {
	uploads        map[string]*fileUpload
	lastUploadScan time.Time
}

// New starts file-operation lifecycle management.
func New(host Host) *Manager {
	if host.LogicalDiskLimitMB == nil {
		host.LogicalDiskLimitMB = func(configured int64) int64 { return configured }
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{Host: host, ctx: ctx, cancel: cancel, operations: make(map[string]*extractionSession), compressions: make(map[string]*compressionSession)}
}

// Close releases incomplete upload buffers under the runtime server-entry lock.
func (s *State) Close() {
	if s == nil {
		return
	}

	for id, item := range s.uploads {
		item.buffer.Close()
		delete(s.uploads, id)
	}
}

// ActiveUploads reports uploads awaiting publication; the caller holds the server lock.
func (s *State) ActiveUploads() bool { return s != nil && len(s.uploads) > 0 }

// Active reports whether a server has unfinished archive work.
func (m *Manager) Active(serverID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.compressionActiveLocked(serverID) {
		return true
	}

	for _, s := range m.operations {
		if s.serverID == serverID {
			s.mu.Lock()
			active := s.busy()
			s.mu.Unlock()

			if active {
				return true
			}
		}
	}

	return false
}

// Close cancels work, waits for writers and removes pending temporary content.
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.cancel()

	operations := make([]*extractionSession, 0, len(m.operations))
	for _, s := range m.operations {
		operations = append(operations, s)
	}
	m.mu.Unlock()

	for _, s := range operations {
		s.cancel()
		s.mu.Lock()
		s.stopTimer()

		if !s.state.Terminal() {
			s.stop(model.ExtractionCancelled, "Spark stopped. Completed entries were kept.")
		}
		s.mu.Unlock()
	}

	m.wg.Wait()

	for _, s := range operations {
		s.mu.Lock()
		s.cleanup()
		s.mu.Unlock()
	}
}

// The caller holds the session lock. Invalidate callbacks already queued by Stop.
func (s *extractionSession) stopTimer() {
	s.timerGeneration++
	if s.timer != nil {
		s.timer.Stop()
	}
}

// Only idle conflict waits expire. Active decoding/copying is cancelled by the
// operation, not by a human-response timer. Reading status never renews it.
func (m *Manager) armExtraction(s *extractionSession) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if m.ctx.Err() != nil {
		return
	}

	s.stopTimer()
	generation := s.timerGeneration

	if s.state == model.ExtractionRunning {
		return
	}

	if s.state.Terminal() {
		s.expires = time.Now().Add(limits.ArchiveResultRetention)
	}

	s.timer = time.AfterFunc(time.Until(s.expires), func() {
		s.mu.Lock()
		if generation != s.timerGeneration {
			s.mu.Unlock()
			return
		}

		if time.Now().Before(s.expires) {
			s.mu.Unlock()
			m.armExtraction(s)

			return
		}

		if s.state == model.ExtractionWaiting {
			s.stop(model.ExtractionExpired, "Extraction expired while waiting for a response. Completed entries were kept.")
			s.mu.Unlock()
			m.armExtraction(s)

			return
		}

		terminal := !s.busy()
		s.mu.Unlock()

		if terminal {
			m.mu.Lock()
			if m.operations[s.id] == s {
				delete(m.operations, s.id)
			}
			m.mu.Unlock()
		}
	})
}
