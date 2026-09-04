package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"vibechat/db"
	"vibechat/middleware"
	"vibechat/models"

	"github.com/gorilla/mux"
)

// ─── Home ─────────────────────────────────────────────────────────────

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r)

	var publicRooms []models.Room
	db.DB.Where("room_type = ?", "public").Order("created_at desc").Find(&publicRooms)

	var myRooms []models.Room
	db.DB.Raw(`SELECT rooms.* FROM rooms 
		INNER JOIN room_members ON rooms.id = room_members.room_id 
		WHERE room_members.user_id = ? AND rooms.room_type = 'private'`, user.ID).Scan(&myRooms)

	var onlineUsers []models.User
	db.DB.Where("is_online = ? AND id != ?", true, user.ID).Limit(10).Find(&onlineUsers)

	renderTemplate(w, r, "home.html", map[string]interface{}{
		"PublicRooms": publicRooms,
		"MyRooms":     myRooms,
		"OnlineUsers": onlineUsers,
	})
}

// ─── Room ─────────────────────────────────────────────────────────────

type MessageWithAuthor struct {
	models.Message
	AuthorUser models.User
}

type RoomPageData struct {
	Room           models.Room
	Messages       []models.Message
	Members        []models.User
	OnlineMembers  []models.User
}

func RoomHandler(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r)
	vars := mux.Vars(r)
	slug := vars["slug"]

	var room models.Room
	if err := db.DB.Preload("Members").Where("slug = ?", slug).First(&room).Error; err != nil {
		http.NotFound(w, r)
		return
	}

	// Auto-join public rooms
	if room.RoomType == "public" {
		// Check if member exists
		var count int64
		db.DB.Model(&models.RoomMember{}).Where("room_id = ? AND user_id = ?", room.ID, user.ID).Count(&count)
		if count == 0 {
			db.DB.Create(&models.RoomMember{RoomID: room.ID, UserID: user.ID})
		}
	} else if room.RoomType == "private" {
		var count int64
		db.DB.Model(&models.RoomMember{}).Where("room_id = ? AND user_id = ?", room.ID, user.ID).Count(&count)
		if count == 0 {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
	}

	// Load members
	var members []models.User
	db.DB.Raw(`SELECT users.* FROM users
		INNER JOIN room_members ON users.id = room_members.user_id
		WHERE room_members.room_id = ?`, room.ID).Scan(&members)

	// Online members
	var onlineMembers []models.User
	db.DB.Raw(`SELECT users.* FROM users
		INNER JOIN room_members ON users.id = room_members.user_id
		WHERE room_members.room_id = ? AND users.is_online = true`, room.ID).Scan(&onlineMembers)

	// Load messages with author and reactions
	var messages []models.Message
	db.DB.Preload("Author").Preload("Reactions").Preload("Reactions.User").
		Where("room_id = ?", room.ID).
		Order("created_at asc").
		Limit(100).
		Find(&messages)

	// Group reactions by emoji for each message
	type ReactionGroup struct {
		Emoji string
		Count int
	}

	type MessageView struct {
		models.Message
		ReactionGroups []ReactionGroup
	}

	var messageViews []MessageView
	for _, msg := range messages {
		groups := map[string]int{}
		for _, rx := range msg.Reactions {
			groups[rx.Emoji]++
		}
		var rgroups []ReactionGroup
		for emoji, count := range groups {
			rgroups = append(rgroups, ReactionGroup{Emoji: emoji, Count: count})
		}
		messageViews = append(messageViews, MessageView{
			Message:        msg,
			ReactionGroups: rgroups,
		})
	}

	renderTemplate(w, r, "room.html", map[string]interface{}{
		"Room":          room,
		"Messages":      messageViews,
		"Members":       members,
		"OnlineMembers": onlineMembers,
		"MembersCount":  len(members),
		"OnlineCount":   len(onlineMembers),
		"UserInitials": user.GetInitials(),
		"CurrentRoomSlug": room.Slug,
	})
}

// ─── Create Room ──────────────────────────────────────────────────────

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9-]`)
var multipleDashes = regexp.MustCompile(`-+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	// Replace spaces with dashes
	s = strings.ReplaceAll(s, " ", "-")
	// Remove non alphanumeric except dashes
	s = nonAlphaNum.ReplaceAllString(s, "")
	// Replace multiple dashes
	s = multipleDashes.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func CreateRoomHandler(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r)
	data := map[string]interface{}{
		"Errors":    map[string]string{},
		"IconChoices": []string{"💬", "🎮", "🎵", "📚", "💻", "🌍", "🎨", "🏆", "🚀", "🎬", "🍕", "⚽"},
	}

	if r.Method == http.MethodPost {
		r.ParseForm()
		name := strings.TrimSpace(r.FormValue("name"))
		roomType := r.FormValue("room_type")
		description := r.FormValue("description")
		icon := r.FormValue("icon")

		errs := map[string]string{}
		if name == "" {
			errs["name"] = "Введіть назву кімнати"
		}
		if roomType == "" {
			roomType = "public"
		}
		if icon == "" {
			icon = "💬"
		}

		if len(errs) == 0 {
			// Generate unique slug
			base := slugify(name)
			if base == "" {
				base = "room"
			}
			slug := base
			for i := 1; ; i++ {
				var count int64
				db.DB.Model(&models.Room{}).Where("slug = ?", slug).Count(&count)
				if count == 0 {
					break
				}
				slug = fmt.Sprintf("%s-%d", base, i)
			}

			room := models.Room{
				Name:        name,
				Slug:        slug,
				Description: description,
				RoomType:    roomType,
				CreatedByID: &user.ID,
				Icon:        icon,
			}
			if err := db.DB.Create(&room).Error; err != nil {
				errs["name"] = "Помилка при створенні кімнати"
			} else {
				db.DB.Create(&models.RoomMember{RoomID: room.ID, UserID: user.ID})
				http.Redirect(w, r, "/room/"+room.Slug+"/", http.StatusFound)
				return
			}
		}
		data["Errors"] = errs
		data["FormName"] = name
		data["FormType"] = roomType
		data["FormDesc"] = description
		data["FormIcon"] = icon
	}

	renderTemplate(w, r, "create_room.html", data)
}

// ─── Users List ───────────────────────────────────────────────────────

func UsersListHandler(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r)
	var users []models.User
	db.DB.Where("id != ?", user.ID).Order("is_online desc, username asc").Find(&users)
	renderTemplate(w, r, "users.html", map[string]interface{}{
		"Users": users,
	})
}

// ─── Upload File ──────────────────────────────────────────────────────

func UploadFileHandler(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r)
	vars := mux.Vars(r)
	roomSlug := vars["room_slug"]

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.ParseMultipartForm(50 << 20) // 50MB

	file, header, err := r.FormFile("file")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false})
		return
	}
	defer file.Close()

	var room models.Room
	if err := db.DB.Where("slug = ?", roomSlug).First(&room).Error; err != nil {
		http.NotFound(w, r)
		return
	}

	uploadDir := "media/chat_files"
	os.MkdirAll(uploadDir, 0755)

	_ = filepath.Ext(header.Filename)
	filename := strings.ReplaceAll(header.Filename, " ", "_")
	fullPath := filepath.Join(uploadDir, filename)

	dst, err := os.Create(fullPath)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false})
		return
	}
	defer dst.Close()
	io.Copy(dst, file)

	relPath := "chat_files/" + filename
	msg := models.Message{
		RoomID:   room.ID,
		AuthorID: user.ID,
		Content:  "📎 " + header.Filename,
		File:     relPath,
		FileName: header.Filename,
		IsFile:   true,
	}
	db.DB.Create(&msg)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"message_id": msg.ID,
		"file_url":   "/media/" + relPath,
		"file_name":  header.Filename,
		"author":     user.Username,
		"timestamp":  msg.CreatedAt.Format("15:04"),
	})
}
