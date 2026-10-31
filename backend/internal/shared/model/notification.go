package model

// NotificationTarget identifies the resource whose access controls an inbox entry.
type NotificationTarget string

// NotificationKind identifies why an inbox message was produced.
type NotificationKind string

// NotificationSeverity determines the message's presentation and delivery priority.
type NotificationSeverity string

// DeliveryState tracks external delivery independently of inbox read state.
type DeliveryState string

// Notification and delivery values retain their persisted and JSON representation.
const (
	NotificationAccount  NotificationTarget   = "account"
	NotificationServer   NotificationTarget   = "server"
	NotificationSpark    NotificationTarget   = "spark"
	NotificationTest     NotificationKind     = "test"
	NotificationAlert    NotificationKind     = "alert"
	NotificationRecovery NotificationKind     = "recovery"
	NotificationBackup   NotificationKind     = "backup"
	NotificationHealth   NotificationKind     = "health"
	NotificationSchedule NotificationKind     = "schedule"
	NotificationInfo     NotificationSeverity = "info"
	NotificationWarning  NotificationSeverity = "warning"
	NotificationFailure  NotificationSeverity = "failure"
	NotificationSuccess  NotificationSeverity = "success"
	DeliveryPending      DeliveryState        = "pending"
	DeliveryDelivered    DeliveryState        = "delivered"
	DeliveryFailed       DeliveryState        = "failed"
)

// AlertCondition distinguishes unavailable observations from healthy and unhealthy ones.
type AlertCondition int

// Conditions distinguish missing observations from successful measurements.
const (
	AlertUnknown   AlertCondition = -1
	AlertHealthy   AlertCondition = 0
	AlertUnhealthy AlertCondition = 1
)
