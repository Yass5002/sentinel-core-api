package database

import (
	"log"

	"github.com/sentinel-app/sentinel/internal/config"
	"github.com/sentinel-app/sentinel/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Connect establishes PostgreSQL connection and runs migrations.
func Connect(cfg *config.Config) *gorm.DB {
	logLevel := logger.Info
	if cfg.GinMode == "release" {
		logLevel = logger.Warn
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("Failed to get underlying sql.DB: %v", err)
	}

	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)

	log.Println("Database connected successfully")
	return db
}

// Migrate runs auto-migrations for all models.
func Migrate(db *gorm.DB) {
	err := db.AutoMigrate(
		&models.Entity{},
		&models.Sequence{},
		&models.Endpoint{},
		&models.Coordinate{},
		&models.Artifact{},
		&models.Connection{},
	)
	if err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations completed")
}
