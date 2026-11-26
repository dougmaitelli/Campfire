package sftp

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"os"
	"slices"
	"time"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

// fileAudit follows one logical request or download handle, never individual
// successful wire chunks. Pending downloads become interrupted on panel recovery.
type fileAudit struct {
	server               *Server
	ctx                  context.Context
	event                database.AuditEvent
	started, unavailable bool
}

func (h *fileHandler) newAudit(action model.AuditAction) *fileAudit {
	if h.server.Audit == nil {
		return nil
	}

	return &fileAudit{server: h.server, ctx: h.ctx, event: database.AuditEvent{
		ID: rand.Text(), Action: action, Target: "server:" + h.id,
		Outcome: model.AuditPending, CreatedAt: time.Now().Unix(),
	}}
}

func (a *fileAudit) actor(p service.Principal) {
	if a == nil {
		return
	}

	if p.UserID != 0 {
		a.event.UserID = &p.UserID
	}

	a.event.Username, a.event.APITokenID = p.Username, p.APITokenID
}

func (a *fileAudit) begin() error {
	if a == nil || a.started {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(a.ctx), 3*time.Second)
	defer cancel()

	err := a.server.Audit.BeginAudit(ctx, &a.event)
	a.started, a.unavailable = err == nil, err != nil

	return err
}

func (a *fileAudit) finish(err error, complete bool) error {
	if a == nil || a.unavailable || !a.started && err == nil && complete {
		return nil
	}

	if !a.started {
		if beginErr := a.begin(); beginErr != nil {
			return beginErr
		}
	}

	a.event.Outcome, a.event.Status = model.AuditSucceeded, 200
	if err != nil {
		a.event.Outcome, a.event.Status = model.AuditFailed, 502
		if errors.Is(err, os.ErrPermission) {
			a.event.Status = 403
		}
	} else if !complete {
		a.event.Outcome = model.AuditInterrupted
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(a.ctx), 3*time.Second)
	defer cancel()

	if err := a.server.Audit.FinishAudit(ctx, a.event); err != nil {
		slog.Error("finish SFTP audit event", "id", a.event.ID, "error", err)
		return err
	}

	return nil
}

// Read offsets may arrive out of order or repeat. Merge coverage rather than
// summing bytes (which could falsely classify a partial download as complete).
// Bound unusual sparse access; uncertain coverage is conservatively interrupted.
type readCoverage struct {
	spans     []readSpan
	uncertain bool
}
type readSpan struct{ start, end int64 }

func (c *readCoverage) add(start, end int64) {
	if c.uncertain || end <= start {
		return
	}

	c.spans = append(c.spans, readSpan{start, end})
	slices.SortFunc(c.spans, func(a, b readSpan) int {
		if a.start < b.start {
			return -1
		}

		if a.start > b.start {
			return 1
		}

		return 0
	})

	merged := c.spans[:0]
	for _, span := range c.spans {
		if len(merged) > 0 && span.start <= merged[len(merged)-1].end {
			merged[len(merged)-1].end = max(merged[len(merged)-1].end, span.end)
		} else {
			merged = append(merged, span)
		}
	}

	c.spans = merged
	if len(c.spans) > 64 {
		c.spans = nil
		c.uncertain = true
	}
}
func (c *readCoverage) complete(size int64) bool {
	return !c.uncertain && (size == 0 || len(c.spans) == 1 && c.spans[0].start == 0 && c.spans[0].end >= size)
}

// Handshake failures are security diagnostics, not an unbounded audit row per
// password attempt. No supplied username, token, or SSH payload is retained.
func (s *Server) authenticationFailure() {
	s.securityMu.Lock()
	defer s.securityMu.Unlock()

	s.securityFailures++
	if time.Since(s.securityLogged) < time.Minute {
		return
	}

	slog.Warn("SFTP authentication denied", "attempts", s.securityFailures)
	s.securityFailures = 0
	s.securityLogged = time.Now()
}
