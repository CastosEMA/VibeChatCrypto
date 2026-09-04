package models

import (
	"strings"
	"time"
)

type User struct {
	ID           uint      `gorm:"primaryKey"`
	Username     string    `gorm:"uniqueIndex;size:150;not null"`
	Email        string    `gorm:"size:254"`
	PasswordHash string    `gorm:"not null"`
	FirstName    string    `gorm:"size:150"`
	LastName     string    `gorm:"size:150"`
	Avatar       string    `gorm:"size:255"` // path relative to media/
	Bio          string    `gorm:"size:300"`
	IsOnline     bool      `gorm:"default:false"`
	LastSeen     *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (u *User) GetInitials() string {
	if u.FirstName != "" && u.LastName != "" {
		return strings.ToUpper(string([]rune(u.FirstName)[0:1]) + string([]rune(u.LastName)[0:1]))
	}
	runes := []rune(u.Username)
	if len(runes) >= 2 {
		return strings.ToUpper(string(runes[0:2]))
	}
	return strings.ToUpper(u.Username)
}

func (u *User) GetAvatarURL() string {
	if u.Avatar != "" {
		return "/media/" + u.Avatar
	}
	return ""
}
