package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"campfire/internal/campfire/database/repository"
	accessperm "campfire/internal/campfire/permissions"
	"campfire/internal/campfire/service"
	"campfire/internal/campfire/statuspage"
	"campfire/internal/shared/model"
)

// WithStatusPages enables administrator publications and their anonymous read endpoint.
func (a *API) WithStatusPages(pages *service.StatusPageService) *API { a.statusPages = pages; return a }

func (a *API) statusPageAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := currentUser(r)
		if !p.Admin || !p.Allows(accessperm.AllAccess) {
			writeError(w, repository.ErrAccessDenied)
			return
		}

		if a.statusPages == nil {
			writeError(w, service.ErrDeploymentUnavailable)
			return
		}

		next(w, r)
	}
}
func (a *API) listStatusPages(w http.ResponseWriter, r *http.Request) {
	pages, err := a.statusPages.List(r.Context(), currentUser(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, pages)
}
func (a *API) saveStatusPage(w http.ResponseWriter, r *http.Request) {
	var input statuspage.Input
	if !decodeRequest(w, r, &input, 32<<10) {
		return
	}

	id := r.PathValue("pageID")

	action := "status-page.update"
	if id == "" {
		action = "status-page.create"
	}

	auditDetail(r, model.AuditAction(action), "status-page:"+safeAuditIdentifier(id))

	page, err := a.statusPages.Save(r.Context(), currentUser(r), id, input)
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "", "status-page:"+page.ID)

	code := http.StatusOK
	if id == "" {
		code = http.StatusCreated
	}

	writeJSON(w, code, page)
}
func (a *API) deleteStatusPage(w http.ResponseWriter, r *http.Request) {
	auditDetail(r, "status-page.delete", "status-page:"+safeAuditIdentifier(r.PathValue("pageID")))

	if err := a.statusPages.Delete(r.Context(), currentUser(r), r.PathValue("pageID")); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Global rate and concurrency bounds also work behind reverse proxies without
// trusting forwarded addresses or retaining an attacker-controlled client map.
func (a *API) publicStatusHandler() http.HandlerFunc {
	var (
		mu     sync.Mutex
		window time.Time
	)

	count := 0
	slots := make(chan struct{}, 16)

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Robots-Tag", "noindex, nofollow")
		mu.Lock()

		now := time.Now()
		if now.Sub(window) >= time.Second {
			window = now
			count = 0
		}

		allowed := count < 100
		if allowed {
			count++
		}
		mu.Unlock()

		if allowed {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			default:
				allowed = false
			}
		}

		if !allowed {
			w.Header().Set("Retry-After", "1")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Too many status requests. Try again shortly."})

			return
		}

		if a.statusPages == nil {
			writeError(w, statuspage.ErrNotFound)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		page, err := a.statusPages.Public(ctx, r.PathValue("slug"))
		if err != nil {
			writeError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, page)
	}
}
