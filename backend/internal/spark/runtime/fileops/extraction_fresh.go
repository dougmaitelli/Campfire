package fileops

import (
	"campfire/internal/shared/model"
	"campfire/internal/shared/workprogress"
	"os"
)

// Fresh-directory extraction uses the same observable lifecycle but does not
// retain one result per member. This preserves the 100,000-member streaming
// limit without growing interactive metadata or command responses unboundedly.
// Both paths write directly; cancellation/failure can leave partial files.
func (m *Manager) scanFreshExtraction(d model.Deployment, s *extractionSession) {
	item := s.items[0]
	ctx := workprogress.WithReporter(s.ctx, func(snapshot workprogress.Snapshot) {
		s.mu.Lock()
		s.progress = &snapshot
		s.version++
		s.mu.Unlock()
	})

	root, err := os.OpenRoot(s.volume)
	if err == nil {
		_, err = m.archiveFiles(ctx, d, root, s.volume, model.FileRequest{ServerID: s.serverID, Operation: model.FileOperationExtract, Path: s.source, Destination: s.destination})
		_ = root.Close()
	}

	s.mu.Lock()
	s.active--
	s.scanDone = true

	if err == nil {
		item.status = model.ExtractionOutcomeExtracted
	} else {
		item.status = model.ExtractionOutcomeFailed

		item.message = ArchiveFailure(err).Error()
		if s.ctx.Err() != nil {
			item.status = model.ExtractionOutcomeCancelled
			item.message = "Extraction stopped. Partial files may remain."
		}

		if !s.state.Terminal() {
			s.stop(model.ExtractionFailed, item.message)
		}
	}

	s.updateState()
	s.version++
	s.mu.Unlock()
	m.armExtraction(s)
}
