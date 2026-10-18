package database

// Schedule defines a recurring server operation. History survives schedule deletion.
type Schedule struct {
	ID              string `gorm:"type:text;primaryKey" json:"id"`
	ServerID        string `gorm:"type:text;not null;index" json:"server_id"`
	Server          Server `gorm:"foreignKey:ServerID;constraint:OnDelete:CASCADE" json:"-"`
	Name            string `gorm:"type:text;not null" json:"name"`
	Action          string `gorm:"type:text;not null" json:"action"`
	Command         string `gorm:"type:text;not null" json:"-"`
	CronExpression  string `gorm:"type:text;not null;default:''" json:"cron_expression"`
	TimeZone        string `gorm:"type:text;not null;default:'UTC'" json:"time_zone"`
	OnlyWhenRunning bool   `gorm:"not null;default:false" json:"only_when_running"`
	NextRunAt       int64  `gorm:"not null;index" json:"next_run_at"`
	Enabled         bool   `gorm:"not null" json:"enabled"`
	RunAsUserID     int64  `gorm:"not null" json:"run_as_user_id"`
	Username        string `gorm:"->;-:migration" json:"username"` // Current user lookup; not a stored schedule column.
	CreatedAt       int64  `gorm:"not null;autoCreateTime" json:"created_at"`
}

// ScheduleRun records manual and recurring operations without foreign-key cascades.
type ScheduleRun struct {
	OnlyWhenRunning bool   `gorm:"not null;default:false" json:"-"`
	ID              string `gorm:"type:text;primaryKey" json:"id"`
	ScheduleID      string `gorm:"type:text;not null;index" json:"schedule_id"`
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
	FinishedAt      int64  `gorm:"not null;index" json:"finished_at"`
	PolledAt        int64  `gorm:"not null;index" json:"-"`
}

// AuditEvent stores allowlisted metadata, never request or response bodies.
type AuditEvent struct {
	APITokenID int64  `gorm:"not null;default:0" json:"api_token_id,omitempty"`
	ID         string `gorm:"type:text;primaryKey" json:"id"`
	UserID     *int64 `gorm:"index" json:"user_id"`
	User       *User  `gorm:"foreignKey:UserID;constraint:OnDelete:RESTRICT" json:"user,omitempty"`
	Username   string `gorm:"type:text;not null" json:"username"`
	Action     string `gorm:"type:text;not null;index" json:"action"`
	Target     string `gorm:"type:text;not null;index" json:"target"`
	Outcome    string `gorm:"type:text;not null" json:"outcome"`
	Status     int    `gorm:"not null" json:"status"`
	CreatedAt  int64  `gorm:"not null;autoCreateTime:false;index" json:"created_at"`
}
