package service

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// HistoryRetention holds independent age limits. Zero keeps a collection forever.
// Configure it before starting ActivityService.
type HistoryRetention struct {
	AuditDays     int
	OperationDays int
}

// Validate rejects negative ages and bounds duration arithmetic to 100 years.
func (r HistoryRetention) Validate() error {
	if r.AuditDays < 0 || r.AuditDays > 36500 || r.OperationDays < 0 || r.OperationDays > 36500 {
		return errors.New("history retention days must be between 0 and 36500 (0 keeps records forever)")
	}

	return nil
}

func (s *ActivityService) pruneHistory(ctx context.Context, now time.Time) (bool, error) {
	var auditBefore, operationsBefore time.Time
	if s.Retention.AuditDays > 0 {
		auditBefore = now.Add(-time.Duration(s.Retention.AuditDays) * 24 * time.Hour)
	}

	if s.Retention.OperationDays > 0 {
		operationsBefore = now.Add(-time.Duration(s.Retention.OperationDays) * 24 * time.Hour)
	}

	return s.Store.PruneHistory(ctx, auditBefore, operationsBefore)
}

func (s *ActivityService) maintainHistory(ctx context.Context) {
	defer s.wg.Done()

	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			batchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			more, err := s.pruneHistory(batchCtx, time.Now())

			cancel()

			if err != nil && ctx.Err() == nil {
				slog.Error("prune activity history", "error", err)
			}

			interval := time.Hour
			if more && err == nil {
				interval = time.Second
			}

			timer.Reset(interval)
		}
	}
}
