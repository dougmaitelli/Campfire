package database

// Notification is an immutable message; read state belongs to each recipient.
type Notification struct {
	ID         int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	TargetType string `gorm:"type:text;not null;index:notification_target,priority:1" json:"target_type"`
	TargetID   string `gorm:"type:text;not null;index:notification_target,priority:2" json:"target_id"`
	Kind       string `gorm:"type:text;not null" json:"kind"`
	Severity   string `gorm:"type:text;not null" json:"severity"`
	Title      string `gorm:"type:text;not null" json:"title"`
	Body       string `gorm:"type:text;not null" json:"body"`
	CreatedAt  int64  `gorm:"not null;autoCreateTime:false" json:"created_at"`
}

// NotificationRecipient snapshots recipients without retaining revoked access.
type NotificationRecipient struct {
	NotificationID int64        `gorm:"primaryKey;autoIncrement:false"`
	Notification   Notification `gorm:"foreignKey:NotificationID;constraint:OnDelete:CASCADE"`
	UserID         int64        `gorm:"primaryKey;autoIncrement:false;index:notification_user_unread,priority:1"`
	User           User         `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	ReadAt         int64        `gorm:"not null;default:0;index:notification_user_unread,priority:2"`
}

// NotificationDelivery queues optional external delivery independently of read state.
type NotificationDelivery struct {
	NotificationID int64        `gorm:"primaryKey;autoIncrement:false"`
	Notification   Notification `gorm:"foreignKey:NotificationID;constraint:OnDelete:CASCADE"`
	State          string       `gorm:"type:text;not null;index:notification_delivery_due,priority:1"`
	Attempts       int          `gorm:"not null"`
	NextAttemptAt  int64        `gorm:"not null;index:notification_delivery_due,priority:2"`
	DeliveredAt    int64        `gorm:"not null"`
	Error          string       `gorm:"type:text;not null"`
}

// AlertState persists transitions so a restart does not repeat active alerts.
type AlertState struct {
	Key            string `gorm:"type:text;primaryKey"`
	TargetType     string `gorm:"type:text;not null;index"`
	TargetID       string `gorm:"type:text;not null;index"`
	Active         bool   `gorm:"not null"`
	Since          int64  `gorm:"not null"`
	RecoverySince  int64  `gorm:"not null"`
	LastObservedAt int64  `gorm:"not null"`
}
