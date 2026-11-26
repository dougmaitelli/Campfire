package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"campfire/internal/campfire/database"
	"campfire/internal/campfire/database/repository"
	"campfire/internal/campfire/service"
	"campfire/internal/shared/model"
)

func auditTestAPI(t *testing.T) (*API, *database.Database) {
	t.Helper()

	db, err := database.Open(context.Background(), filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	return New(nil, nil, nil).WithActivity(service.NewActivityService(repository.NewActivityRepository(db.ORM()), nil, nil)), db
}

func TestQuietAuditPoliciesRetainFailures(t *testing.T) {
	for _, route := range []string{
		"PUT /api/v1/notifications/{notificationID}/read", "POST /api/v1/notifications/read-all",
		"POST /api/v1/schedules/preview", "POST /api/v1/embers/preview", "POST /api/v1/integrations/discord/preview",
		"POST /api/v1/backup-imports/{token}/chunks",
	} {
		t.Run(route, func(t *testing.T) {
			a, db := auditTestAPI(t)
			status := http.StatusOK
			mux := http.NewServeMux()
			mux.HandleFunc(route, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
			handler := a.auditRequests(mux, mux)
			method, path, _ := strings.Cut(route, " ")
			path = strings.ReplaceAll(strings.ReplaceAll(path, "{notificationID}", "1"), "{token}", "private-upload-token")

			for _, code := range []int{200, 403, 500} {
				status = code

				handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), method, path, nil))
			}

			var rows []database.AuditEvent
			if err := db.ORM().Find(&rows).Error; err != nil {
				t.Fatal(err)
			}

			if len(rows) != 2 {
				t.Fatalf("expected failures only, got %+v", rows)
			}

			for _, row := range rows {
				if row.Outcome != model.AuditFailed || strings.Contains(row.Target, "private-upload-token") {
					t.Fatal(row)
				}
			}
		})
	}
}

func TestFileAuditFailsClosedOnlyAtLogicalBoundaries(t *testing.T) {
	for _, operation := range []model.FileOperation{model.FileOperationWrite, model.FileOperationReadChunk, model.FileOperationUploadCommit, model.FileOperationExtractStart, model.FileOperationUploadChunk, model.FileOperationCompressStatus} {
		t.Run(string(operation), func(t *testing.T) {
			a, db := auditTestAPI(t)
			if err := db.ORM().Migrator().DropTable(&database.AuditEvent{}); err != nil {
				t.Fatal(err)
			}

			called := false
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/v1/servers/{id}/files", func(w http.ResponseWriter, r *http.Request) {
				if !auditFileRequest(w, r, operation) {
					return
				}

				called = true

				w.WriteHeader(http.StatusOK)
			})

			response := httptest.NewRecorder()
			a.auditRequests(mux, mux).ServeHTTP(response, httptest.NewRequestWithContext(t.Context(), "POST", "/api/v1/servers/game/files", nil))

			quiet := routineFileOperation(operation)
			if called != quiet || !quiet && response.Code != http.StatusServiceUnavailable {
				t.Fatal(called, response.Code)
			}
		})
	}
}

type failedAuditWriter struct {
	*httptest.ResponseRecorder
	short bool
}

func (w failedAuditWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}

	return 0, io.ErrClosedPipe
}

func TestDownloadAuditCompletionAndWriteFailures(t *testing.T) {
	for _, mode := range []string{"complete", "partial", "write-error", "short-write", "denied", "panic", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			a, db := auditTestAPI(t)
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v1/backups/{backupID}", func(w http.ResponseWriter, r *http.Request) {
				if mode == "denied" {
					w.WriteHeader(403)
					return
				}

				if mode == "panic" {
					panic(http.ErrAbortHandler)
				}

				_, _ = w.Write([]byte("data"))

				if mode != "partial" {
					auditDownloadComplete(r)
				}
			})

			var w http.ResponseWriter = httptest.NewRecorder()
			if mode == "write-error" || mode == "short-write" {
				w = failedAuditWriter{httptest.NewRecorder(), mode == "short-write"}
			}

			req := httptest.NewRequestWithContext(t.Context(), "GET", "/api/v1/backups/archive", nil)
			if mode == "cancelled" {
				ctx, cancel := context.WithCancel(req.Context())
				cancel()

				req = req.WithContext(ctx)
			}

			func() {
				defer func() {
					if p := recover(); p != nil {
						err, ok := p.(error)
						if !ok || !errors.Is(err, http.ErrAbortHandler) {
							panic(p)
						}
					}
				}()

				a.auditRequests(mux, mux).ServeHTTP(w, req)
			}()

			var rows []database.AuditEvent
			if err := db.ORM().Find(&rows).Error; err != nil {
				t.Fatal(err)
			}

			want := model.AuditInterrupted
			if mode == "complete" {
				want = model.AuditSucceeded
			}

			if mode == "denied" {
				want = model.AuditFailed
			}

			if len(rows) != 1 || rows[0].Outcome != want || rows[0].Action != "server.backup.download" || rows[0].Target != "backup:archive" {
				t.Fatal(rows)
			}
		})
	}
}

func TestSensitiveAuditRouteTargets(t *testing.T) {
	for _, tc := range []struct{ pattern, path, target, want string }{
		{"DELETE /api/v1/servers/{id}/access/grants/{grantID}", "/api/v1/servers/game/access/grants/4", "server:game", "server:game grant:4"},
		{"DELETE /api/v1/servers/{id}/members/{userID}", "/api/v1/servers/game/members/8", "server:game", "server:game user:8"},
		{"GET /api/v1/servers/{id}/backups/{name}", "/api/v1/servers/game/backups/game.tar.gz", "server:game", "server:game backup:game.tar.gz"},
	} {
		method, _, _ := strings.Cut(tc.pattern, " ")
		if got := auditRouteTarget(httptest.NewRequestWithContext(t.Context(), method, tc.path, nil), tc.pattern, tc.target); got != tc.want {
			t.Fatal(got, tc.want)
		}

		if auditPolicy(tc.pattern).action == "" {
			t.Fatal("missing semantic action", tc.pattern)
		}
	}

	if auditPolicy("POST /api/v1/embers/preview-url").quiet {
		t.Fatal("remote URL preview must remain audited")
	}
}
