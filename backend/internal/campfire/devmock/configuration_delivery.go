//go:build !production

package devmock

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Runtime) checkConfigurationDelivery(ctx context.Context, spark string) error {
	var client sparkClientRecord
	if err := r.db.WithContext(ctx).Take(&client, "spark_id = ?", spark).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}

	if client.ConfigurationBlocked {
		return errors.New("mock Spark configuration delivery is paused; run mock-config resume in Deployment sync · Mock's console")
	}

	return nil
}

func (r *Runtime) configurationCommand(ctx context.Context, spark, command string) error {
	if !strings.HasPrefix(command, "mock-config ") {
		return nil
	}

	var blocked bool

	switch strings.TrimPrefix(command, "mock-config ") {
	case "pause":
		blocked = true
	case "resume":
	default:
		return errors.New("mock-config expects pause or resume")
	}

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "spark_id"}}, DoUpdates: clause.AssignmentColumns([]string{"configuration_blocked"}),
	}).Create(&sparkClientRecord{SparkID: spark, Version: "mock-dev", ConfigurationBlocked: blocked}).Error
}
