package repository

import (
	"campfire/internal/campfire/database"
	"campfire/internal/shared/model"

	"gorm.io/gorm"
)

const serverNameConflictMessage = "A server with this name already exists."

type serverConflictError string

func (e serverConflictError) Error() string { return string(e) }
func (serverConflictError) Unwrap() error   { return ErrServerConflict }

func checkServerName(tx *gorm.DB, server model.Server) error {
	var count int64
	if err := tx.Model(&database.Server{}).Where("lower(name) = lower(?) AND id <> ?", server.Name, server.ID).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		return serverConflictError(serverNameConflictMessage)
	}

	return nil
}
