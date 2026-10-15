package database

// Schedule defines a recurring server operation. History survives schedule deletion.
type Schedule struct {
	ID              string `gorm:"type:text;primaryKey" json:"id"`
	ServerID        string `gorm:"type:text;not null;index" json:"server_id"`
	Server          Server `gorm:"foreignKey:ServerID;constraint:OnDelete:CASCADE" json:"-"`
	Name            string `gorm:"type:text;not null" json:"name"`
	Action          string `gorm:"type:text;not null" json:"action"`
	Command         string `gorm:"type:text;not null" json:"-"`
	IntervalMinutes int64  `gorm:"not null" json:"interval_minutes"`
	NextRunAt       int64  `gorm:"not null;index" json:"next_run_at"`
	Enabled         bool   `gorm:"not null" json:"enabled"`
	UserID          int64  `gorm:"not null" json:"user_id"`
	Username        string `gorm:"type:text;not null" json:"username"`
	CreatedAt       int64  `gorm:"not null;autoCreateTime" json:"created_at"`
	Version         int64  `gorm:"not null" json:"version"`
}

// ScheduleRun records manual and recurring operations without foreign-key cascades.
type ScheduleRun struct {
	ID              string `gorm:"type:text;primaryKey" json:"id"`
	ScheduleID      string `gorm:"type:text;not null;index" json:"schedule_id"`
	ScheduleVersion int64  `gorm:"not null" json:"-"`
	Recurring       bool   `gorm:"not null" json:"recurring"`
	RetryOf         string `gorm:"type:text;not null" json:"retry_of"`
	ServerID        string `gorm:"type:text;not null;index" json:"server_id"`
	ServerName      string `gorm:"type:text;not null" json:"server_name"`
	UserID          int64  `gorm:"not null" json:"user_id"`
	Username        string `gorm:"type:text;not null" json:"username"`
	Action          string `gorm:"type:text;not null" json:"action"`
	Content         string `gorm:"type:text;not null" json:"-"`
	State           string `gorm:"type:text;not null;index" json:"state"`
	Message         string `gorm:"type:text;not null" json:"message"`
	CreatedAt       int64  `gorm:"not null;autoCreateTime:false;index" json:"created_at"`
	StartedAt       int64  `gorm:"not null" json:"started_at"`
	FinishedAt      int64  `gorm:"not null" json:"finished_at"`
	PolledAt        int64  `gorm:"not null;index" json:"-"`
}

// AuditEvent stores allowlisted metadata, never request or response bodies.
type AuditEvent struct {
	APITokenID int64  `gorm:"not null;default:0" json:"api_token_id,omitempty"`
	ID         string `gorm:"type:text;primaryKey" json:"id"`
	UserID     int64  `gorm:"not null;index" json:"user_id"`
	Username   string `gorm:"type:text;not null" json:"username"`
	Action     string `gorm:"type:text;not null;index" json:"action"`
	Target     string `gorm:"type:text;not null;index" json:"target"`
	Outcome    string `gorm:"type:text;not null" json:"outcome"`
	Status     int    `gorm:"not null" json:"status"`
	CreatedAt  int64  `gorm:"not null;autoCreateTime:false;index" json:"created_at"`
}
