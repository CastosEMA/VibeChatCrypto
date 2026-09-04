package models

import "time"

type Room struct {
	ID          uint   `gorm:"primaryKey"`
	Name        string `gorm:"uniqueIndex;size:100;not null"`
	Slug        string `gorm:"uniqueIndex;size:100;not null"`
	Description string
	RoomType    string `gorm:"size:10;default:'public'"` // public, private, direct
	CreatedByID *uint
	CreatedBy   *User `gorm:"foreignKey:CreatedByID"`
	Members     []User `gorm:"many2many:room_members;"`
	Icon        string `gorm:"size:10;default:'💬'"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RoomMember is the join table for Room.Members
type RoomMember struct {
	RoomID uint `gorm:"primaryKey"`
	UserID uint `gorm:"primaryKey"`
}

func (r *Room) GetMembersCount(members []User) int {
	return len(members)
}

type Message struct {
	ID        uint   `gorm:"primaryKey"`
	RoomID    uint   `gorm:"not null;index"`
	Room      Room   `gorm:"foreignKey:RoomID"`
	AuthorID  uint   `gorm:"not null"`
	Author    User   `gorm:"foreignKey:AuthorID"`
	Content   string `gorm:"not null"`
	File      string `gorm:"size:255"` // path relative to media/
	FileName  string `gorm:"size:255"`
	IsFile    bool   `gorm:"default:false"`
	IsEdited  bool   `gorm:"default:false"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Reactions []Reaction `gorm:"foreignKey:MessageID"`
}

func (m *Message) GetFileURL() string {
	if m.File != "" {
		return "/media/" + m.File
	}
	return ""
}

type Reaction struct {
	ID        uint   `gorm:"primaryKey"`
	MessageID uint   `gorm:"not null;index"`
	UserID    uint   `gorm:"not null"`
	User      User   `gorm:"foreignKey:UserID"`
	Emoji     string `gorm:"size:10;not null"`
	CreatedAt time.Time
}
