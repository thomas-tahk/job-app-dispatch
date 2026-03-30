package db

import (
	"github.com/thomas-tahk/job-app-dispatch/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Open creates (or opens) the SQLite database at path and runs auto-migrations.
func Open(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&models.Job{},
		&models.Application{},
		&models.Resume{},
	); err != nil {
		return nil, err
	}
	return db, nil
}
