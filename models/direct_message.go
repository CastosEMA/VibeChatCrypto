package models

import "time"

type DirectMessage struct {
	ID         uint `gorm:"primaryKey"`
	SenderID   uint `gorm:"not null;index"`
	Sender     User `gorm:"foreignKey:SenderID"`
	ReceiverID uint `gorm:"not null;index"`
	Receiver   User `gorm:"foreignKey:ReceiverID"`
	Content    string
	IsRead     bool `gorm:"default:false"`
	CreatedAt  time.Time
}
