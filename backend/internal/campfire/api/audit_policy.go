package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"campfire/internal/campfire/database"
	accessperm "campfire/internal/campfire/permissions"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

// Policies describe logical actions, not HTTP verbs. Quiet requests retain failures
// but do not require an audit write before routine polling or private UI changes.
type requestAuditPolicy struct {
	action                 model.AuditAction
	quiet, files, download bool
}

func auditPolicy(pattern string) requestAuditPolicy {
	switch pattern {
	case "GET /api/v1/backups/{backupID}", "GET /api/v1/servers/{id}/backups/{name}":
		return requestAuditPolicy{action: "server.backup.download", download: true}
	case "GET /api/v1/servers/{id}/files/download":
		return requestAuditPolicy{action: "server.files.download", download: true}
	case "POST /api/v1/servers/{id}/files":
		return requestAuditPolicy{files: true}
	case "PUT /api/v1/notifications/{notificationID}/read", "POST /api/v1/notifications/read-all":
		return requestAuditPolicy{action: "notification.read", quiet: true}
	case "POST /api/v1/schedules/preview":
		return requestAuditPolicy{action: "schedule.preview", quiet: true}
	case "POST /api/v1/embers/preview":
		return requestAuditPolicy{action: "ember.preview", quiet: true}
	case "POST /api/v1/integrations/discord/preview":
		return requestAuditPolicy{action: "integration.discord.preview", quiet: true}
	case "POST /api/v1/backup-imports/{token}/chunks":
		return requestAuditPolicy{action: "backup.import.chunk", quiet: true}
	case "POST /api/v1/backup-imports":
		return requestAuditPolicy{action: "backup.import.begin"}
	case "POST /api/v1/backup-imports/{token}/inspect":
		return requestAuditPolicy{action: "backup.import.inspect"}
	case "DELETE /api/v1/backup-imports/{token}":
		return requestAuditPolicy{action: "backup.import.cancel"}
	case "POST /api/v1/backups/{backupID}/prepare":
		return requestAuditPolicy{action: "backup.prepare"}
	case "POST /api/v1/embers/preview-url":
		return requestAuditPolicy{action: "ember.preview-url"}
	case "POST /api/v1/servers/{id}/console":
		return requestAuditPolicy{action: "server.command"}
	case "POST /api/v1/servers/{id}/access/grants":
		return requestAuditPolicy{action: "server.access.grant"}
	case "DELETE /api/v1/servers/{id}/access/grants/{grantID}":
		return requestAuditPolicy{action: "server.access.revoke"}
	case "POST /api/v1/servers/{id}/members":
		return requestAuditPolicy{action: "server.member.add"}
	case "DELETE /api/v1/servers/{id}/members/{userID}":
		return requestAuditPolicy{action: "server.member.remove"}
	case "PUT /api/v1/servers/{id}/owner":
		return requestAuditPolicy{action: "server.owner.transfer"}
	case "POST /api/v1/sparks/{id}/access/grants":
		return requestAuditPolicy{action: "spark.access.grant"}
	case "DELETE /api/v1/sparks/{id}/access/grants/{grantID}":
		return requestAuditPolicy{action: "spark.access.revoke"}
	case "PUT /api/v1/sparks/{id}/roles":
		return requestAuditPolicy{action: "spark.roles.set"}
	}

	return requestAuditPolicy{}
}

type auditRequestKey struct{}
type auditRequest struct {
	event                            *database.AuditEvent
	activity                         *service.ActivityService
	started, quiet, downloadComplete bool
}

func (s *auditRequest) begin(r *http.Request) error {
	if s.started {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
	defer cancel()

	if err := s.activity.Store.BeginAudit(ctx, s.event); err != nil {
		return err
	}

	s.started = true

	return nil
}

func auditFileRequest(w http.ResponseWriter, r *http.Request, operation model.FileOperation) bool {
	state, ok := r.Context().Value(auditRequestKey{}).(*auditRequest)
	if !ok {
		return true
	}

	state.quiet = routineFileOperation(operation)

	action := string(operation)
	if accessperm.File(operation) == "" {
		action = "invalid"
	}

	auditDetail(r, model.ResourceAuditAction("server.files", action), "")

	if !state.quiet {
		if err := state.begin(r); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "audit storage unavailable; action was not performed"})
			return false
		}
	}

	return true
}

func routineFileOperation(operation model.FileOperation) bool {
	switch operation {
	// A standalone public read.chunk is still a data read: unlike internal
	// download chunks, it has no enclosing audited transfer.
	case model.FileOperationList, model.FileOperationStat, model.FileOperationUploadChunk, model.FileOperationCompressStatus, model.FileOperationExtractStatus:
		return true
	}

	return false
}

func auditDownloadComplete(r *http.Request) {
	if state, ok := r.Context().Value(auditRequestKey{}).(*auditRequest); ok {
		state.downloadComplete = true
	}
}

// Only named resource identifiers are retained; upload/session tokens are never targets.
func auditRouteTarget(r *http.Request, pattern, target string) string {
	route := strings.Split(strings.TrimPrefix(strings.TrimPrefix(pattern, r.Method+" "), "/"), "/")

	parts := strings.Split(strings.TrimPrefix(r.URL.EscapedPath(), "/"), "/")
	if len(route) != len(parts) {
		return target
	}

	for i, part := range route {
		label := ""

		switch part {
		case "{grantID}":
			label = "grant"
		case "{userID}":
			if strings.HasPrefix(target, "server:") {
				label = "user"
			}
		case "{name}":
			if strings.Contains(pattern, "/backups/") {
				label = "backup"
			}
		}

		if label == "" {
			continue
		}

		value, err := url.PathUnescape(parts[i])
		if err == nil && safeAuditIdentifier(value) != "" {
			target += " " + label + ":" + value
		}
	}

	return target
}

func auditGrantTarget(r *http.Request, kind string, input service.AccessGrantInput) {
	target := kind + ":" + r.PathValue("id")
	if name := safeAuditIdentifier(strings.ToLower(strings.TrimSpace(input.Username))); name != "" {
		target += " username:" + name
	}

	if input.RoleID != nil {
		target += " role:" + strconv.FormatInt(*input.RoleID, 10)
	}

	target += " access_role:" + strconv.FormatInt(input.AccessRoleID, 10)
	auditDetail(r, "", target)
}

func safeAuditIdentifier(value string) string {
	if value == "" || len(value) > 128 || strings.IndexFunc(value, func(r rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-.@", r)
	}) != -1 {
		return ""
	}

	return value
}
