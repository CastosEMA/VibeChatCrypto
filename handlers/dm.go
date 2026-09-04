package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"vibechat/db"
	"vibechat/middleware"
	"vibechat/models"

	"github.com/gorilla/mux"
)

func DirectMessageHandler(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r)
	vars := mux.Vars(r)
	username := vars["username"]

	var otherUser models.User
	if err := db.DB.Where("username = ?", username).First(&otherUser).Error; err != nil {
		http.NotFound(w, r)
		return
	}

	// Create or get DM room slug (sorted IDs)
	ids := []uint{user.ID, otherUser.ID}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	roomSlug := fmt.Sprintf("dm-%d-%d", ids[0], ids[1])

	var room models.Room
	result := db.DB.Where("slug = ?", roomSlug).First(&room)
	if result.Error != nil {
		// Create DM room
		room = models.Room{
			Name:        fmt.Sprintf("DM: %s & %s", user.Username, otherUser.Username),
			Slug:        roomSlug,
			RoomType:    "direct",
			CreatedByID: &user.ID,
			Icon:        "💬",
		}
		db.DB.Create(&room)
		// Add members (ignore duplicate errors)
		db.DB.Exec("INSERT OR IGNORE INTO room_members (room_id, user_id) VALUES (?, ?)", room.ID, user.ID)
		db.DB.Exec("INSERT OR IGNORE INTO room_members (room_id, user_id) VALUES (?, ?)", room.ID, otherUser.ID)
	}

	// Load messages
	var messages []models.Message
	db.DB.Preload("Author").
		Where("room_id = ?", room.ID).
		Order("created_at asc").
		Limit(100).
		Find(&messages)

	renderTemplate(w, r, "dm.html", map[string]interface{}{
		"Room":      room,
		"Messages":  messages,
		"OtherUser": otherUser,
		"OtherUserInitials": otherUser.GetInitials(),
		"UserInitials": user.GetInitials(),
	})
}
