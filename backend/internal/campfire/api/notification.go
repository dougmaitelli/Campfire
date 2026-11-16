package api

import (
	"net/http"
	"strconv"
	"time"

	"campfire/internal/campfire/service"
)

func (a *API) notificationsEnabled(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.notifications == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "notifications unavailable"})
			return
		}

		next(w, r)
	}
}

func (a *API) listNotifications(w http.ResponseWriter, r *http.Request) {
	request, ok := pageRequest(w, r)
	if !ok {
		return
	}

	items, err := a.notifications.Page(r.Context(), currentUser(r), r.URL.Query().Get("unread") == "true", request)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func (a *API) unreadNotifications(w http.ResponseWriter, r *http.Request) {
	count, err := a.notifications.Unread(r.Context(), currentUser(r))
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]int64{"unread": count})
}

func (a *API) readNotification(w http.ResponseWriter, r *http.Request) {
	id, ok := pathNumber(w, r, "notificationID")
	if !ok {
		return
	}

	var input struct {
		Read bool `json:"read"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	if err := a.notifications.MarkRead(r.Context(), currentUser(r), id, input.Read); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) readAllNotifications(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ThroughID int64 `json:"through_id"`
	}
	if !decodeRequest(w, r, &input, 1024) {
		return
	}

	if input.ThroughID < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid notification watermark"})
		return
	}

	if err := a.notifications.MarkAllRead(r.Context(), currentUser(r), input.ThroughID); err != nil {
		writeError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) notificationStatus(w http.ResponseWriter, r *http.Request) {
	pending, failed, err := a.notifications.Store.DeliveryCounts(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}

	config := a.notifications.Config

	interval := service.DefaultResourceInterval
	if a.deployment != nil && a.deployment.History != nil {
		interval = a.deployment.History.Interval
	}

	writeJSON(w, http.StatusOK, map[string]any{"apprise_enabled": a.notifications.Apprise != nil, "pending": pending, "failed": failed, "cpu_percent": config.CPUPercent, "memory_percent": config.MemoryPercent, "disk_percent": config.DiskPercent, "duration_seconds": int64(config.Duration.Seconds()), "interval_seconds": int64(interval.Seconds()), "health_enabled": config.Health})
}

func (a *API) testNotification(w http.ResponseWriter, r *http.Request) {
	id, err := a.notifications.Test(r.Context(), currentUser(r))
	if err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "notification.test", "notification:"+strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusAccepted, map[string]any{"notification_id": id, "apprise_queued": a.notifications.Apprise != nil})
}

func (a *API) retryNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	if err := a.notifications.Store.RetryDeliveries(r.Context(), time.Now()); err != nil {
		writeError(w, err)
		return
	}

	auditDetail(r, "notification.retry-deliveries", "notifications")
	w.WriteHeader(http.StatusNoContent)
}
