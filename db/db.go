package db

import (
	"log"
	"vibechat/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Init(dbPath string) {
	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	err = DB.AutoMigrate(
		&models.User{},
		&models.Room{},
		&models.RoomMember{},
		&models.Message{},
		&models.Reaction{},
		&models.DirectMessage{},
	)
	if err != nil {
		log.Fatalf("Failed to auto-migrate: %v", err)
	}

	log.Println("Database initialized")
}
