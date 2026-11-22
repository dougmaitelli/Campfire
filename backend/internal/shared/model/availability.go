package model

// Availability history uses five-minute intervals over the last completed day.
// Durations are observed coverage, not an external connectivity or SLA test.
const (
	AvailabilityIntervalSeconds int64 = 300
	AvailabilityHistoryPoints         = 288
)

// AvailabilityPoint retains known coverage; the rest of the interval is unknown.
type AvailabilityPoint struct {
	OnlineSeconds  int64 `json:"online_seconds"`
	OfflineSeconds int64 `json:"offline_seconds"`
}

// AvailabilityHistory is a bounded, identifier-free public projection.
type AvailabilityHistory struct {
	From     int64               `json:"from"`
	To       int64               `json:"to"`
	Interval int64               `json:"interval"`
	Points   []AvailabilityPoint `json:"points"`
}

// EmptyAvailabilityHistory leaves missing observations unknown, never offline.
func EmptyAvailabilityHistory(now int64) AvailabilityHistory {
	to := now / AvailabilityIntervalSeconds * AvailabilityIntervalSeconds
	return AvailabilityHistory{From: to - AvailabilityIntervalSeconds*AvailabilityHistoryPoints, To: to, Interval: AvailabilityIntervalSeconds, Points: make([]AvailabilityPoint, AvailabilityHistoryPoints)}
}
