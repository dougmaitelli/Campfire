package repository

import (
	"context"
	"errors"
	"time"

	"campfire/internal/campfire/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrEnrollment identifies an invalid token or ineligible Spark.
var ErrEnrollment = errors.New("invalid or expired enrollment")

// EnrollmentRepository handles atomic enrollment and certificate authorization.
type EnrollmentRepository struct{ db *gorm.DB }

// NewEnrollmentRepository creates an enrollment store.
func NewEnrollmentRepository(db *gorm.DB) *EnrollmentRepository { return &EnrollmentRepository{db: db} }

// CreateToken replaces outstanding enrollment tokens for an enabled remote Spark.
func (r *EnrollmentRepository) CreateToken(ctx context.Context, id, hash string, expires time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var node database.Spark
		if err := tx.Where("id = ?", id).Take(&node).Error; err != nil {
			return err
		}

		if node.Kind != "remote" || node.Disabled {
			return ErrEnrollment
		}

		if err := tx.Where("spark_id = ? OR expires_at <= ?", id, time.Now().Unix()).Delete(&database.SparkEnrollment{}).Error; err != nil {
			return err
		}

		return tx.Create(&database.SparkEnrollment{Hash: hash, SparkID: id, ExpiresAt: expires.Unix()}).Error
	})
}

// Redeem atomically signs an identity and consumes its enrollment token.
func (r *EnrollmentRepository) Redeem(ctx context.Context, hash string, sign func(string) (string, error)) (string, error) {
	var id string

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var token database.SparkEnrollment
		if err := tx.Preload("Spark").Where("hash = ?", hash).Take(&token).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEnrollment
			}

			return err
		}

		if token.ExpiresAt <= time.Now().Unix() || token.Spark.Disabled || token.Spark.Kind != "remote" {
			return ErrEnrollment
		}

		certificateHash, err := sign(token.SparkID)
		if err != nil {
			return err
		}

		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "spark_id"}}, DoUpdates: clause.AssignmentColumns([]string{"certificate_hash"})}).Create(&database.SparkIdentity{SparkID: token.SparkID, CertificateHash: certificateHash}).Error; err != nil {
			return err
		}

		if err := tx.Where("hash = ?", hash).Delete(&database.SparkEnrollment{}).Error; err != nil {
			return err
		}

		id = token.SparkID

		return nil
	})

	return id, err
}

// Authorized checks the current identity and administrative node state.
func (r *EnrollmentRepository) Authorized(ctx context.Context, id, hash string) bool {
	var count int64

	err := r.db.WithContext(ctx).Model(&database.SparkIdentity{}).Joins("JOIN sparks ON sparks.id = spark_id").Where("spark_id = ? AND certificate_hash = ? AND sparks.disabled = ? AND sparks.kind = ?", id, hash, false, "remote").Count(&count).Error

	return err == nil && count == 1
}
