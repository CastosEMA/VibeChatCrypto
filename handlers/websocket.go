package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
	"vibechat/db"
	"vibechat/middleware"
	"vibechat/models"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ─── Hub ──────────────────────────────────────────────────────────────

// Hub manages all active WebSocket clients grouped by room slug.
type Hub struct {
	mu      sync.RWMutex
	rooms   map[string]map[*Client]bool // roomSlug → clients
	online  map[*Client]bool            // online-status clients
	broadcast chan hubMessage
}

type hubMessage struct {
	roomSlug string
	payload  []byte
	online   bool // true = broadcast to online-status group
}

var GlobalHub = &Hub{
	rooms:     make(map[string]map[*Client]bool),
	online:    make(map[*Client]bool),
	broadcast: make(chan hubMessage, 256),
}

func (h *Hub) Run() {
	for msg := range h.broadcast {
		h.mu.RLock()
		if msg.online {
			for c := range h.online {
				select {
				case c.send <- msg.payload:
				default:
					close(c.send)
				}
			}
		} else {
			for c := range h.rooms[msg.roomSlug] {
				select {
				case c.send <- msg.payload:
				default:
					close(c.send)
				}
			}
		}
		h.mu.RUnlock()
	}
}

func (h *Hub) addRoom(slug string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[slug] == nil {
		h.rooms[slug] = make(map[*Client]bool)
	}
	h.rooms[slug][c] = true
}

func (h *Hub) removeRoom(slug string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients, ok := h.rooms[slug]; ok {
		delete(clients, c)
		if len(clients) == 0 {
			delete(h.rooms, slug)
		}
	}
}

func (h *Hub) addOnline(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.online[c] = true
}

func (h *Hub) removeOnline(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.online, c)
}

// ─── Client ───────────────────────────────────────────────────────────

type Client struct {
	conn     *websocket.Conn
	send     chan []byte
	user     *models.User
	roomSlug string
	isOnline bool // true = OnlineStatusConsumer
}

func (c *Client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

// ─── Chat WebSocket ───────────────────────────────────────────────────

func ChatWSHandler(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetCurrentUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	roomSlug := vars["room_slug"]

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WS upgrade error:", err)
		return
	}

	client := &Client{
		conn:     conn,
		send:     make(chan []byte, 256),
		user:     user,
		roomSlug: roomSlug,
	}

	GlobalHub.addRoom(roomSlug, client)
	setUserOnline(user.ID, true)
	go client.writePump()

	// Notify room of new user
	broadcastStatus(roomSlug, user, "online", false)

	// Read pump
	defer func() {
		GlobalHub.removeRoom(roomSlug, client)
		setUserOnline(user.ID, false)
		broadcastStatus(roomSlug, user, "offline", false)
		conn.Close()
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var data map[string]interface{}
		if err := json.Unmarshal(msgBytes, &data); err != nil {
			continue
		}

		msgType, _ := data["type"].(string)

		switch msgType {
		case "message":
			content, _ := data["content"].(string)
			if content == "" {
				continue
			}
			msg := saveMessage(roomSlug, user, content)
			if msg == nil {
				continue
			}
			payload, _ := json.Marshal(map[string]interface{}{
				"type":       "message",
				"message_id": msg.ID,
				"content":    content,
				"author":     user.Username,
				"author_id":  user.ID,
				"initials":   user.GetInitials(),
				"timestamp":  msg.CreatedAt.Format("15:04"),
				"date":       msg.CreatedAt.Format("02.01.2006"),
			})
			GlobalHub.broadcast <- hubMessage{roomSlug: roomSlug, payload: payload}

		case "typing":
			isTyping, _ := data["is_typing"].(bool)
			payload, _ := json.Marshal(map[string]interface{}{
				"type":      "typing",
				"username":  user.Username,
				"is_typing": isTyping,
			})
			// broadcast to room but skip sender on client side
			GlobalHub.broadcast <- hubMessage{roomSlug: roomSlug, payload: payload}

		case "reaction":
			msgIDFloat, _ := data["message_id"].(float64)
			emoji, _ := data["emoji"].(string)
			if msgIDFloat == 0 || emoji == "" {
				continue
			}
			msgID := uint(msgIDFloat)
			count := toggleReaction(msgID, user.ID, emoji)
			payload, _ := json.Marshal(map[string]interface{}{
				"type":       "reaction",
				"message_id": msgID,
				"emoji":      emoji,
				"count":      count,
				"user_id":    user.ID,
			})
			GlobalHub.broadcast <- hubMessage{roomSlug: roomSlug, payload: payload}
		}
	}
}

// ─── Online Status WebSocket ──────────────────────────────────────────

func OnlineStatusWSHandler(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetCurrentUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := &Client{
		conn:     conn,
		send:     make(chan []byte, 64),
		user:     user,
		isOnline: true,
	}

	GlobalHub.addOnline(client)
	setUserOnline(user.ID, true)
	go client.writePump()

	payload, _ := json.Marshal(map[string]interface{}{
		"type":     "status_update",
		"user_id":  user.ID,
		"username": user.Username,
		"status":   "online",
	})
	GlobalHub.broadcast <- hubMessage{online: true, payload: payload}

	defer func() {
		GlobalHub.removeOnline(client)
		setUserOnline(user.ID, false)
		p, _ := json.Marshal(map[string]interface{}{
			"type":     "status_update",
			"user_id":  user.ID,
			"username": user.Username,
			"status":   "offline",
		})
		GlobalHub.broadcast <- hubMessage{online: true, payload: p}
		conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────

func broadcastStatus(roomSlug string, user *models.User, status string, isOnline bool) {
	payload, _ := json.Marshal(map[string]interface{}{
		"type":     "user_status",
		"user_id":  user.ID,
		"username": user.Username,
		"status":   status,
		"initials": user.GetInitials(),
	})
	GlobalHub.broadcast <- hubMessage{roomSlug: roomSlug, payload: payload, online: isOnline}
}

func setUserOnline(userID uint, online bool) {
	now := time.Now()
	db.DB.Model(&models.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"is_online": online,
		"last_seen": now,
	})
}

func saveMessage(roomSlug string, user *models.User, content string) *models.Message {
	var room models.Room
	if err := db.DB.Where("slug = ?", roomSlug).First(&room).Error; err != nil {
		return nil
	}
	msg := &models.Message{
		RoomID:   room.ID,
		AuthorID: user.ID,
		Content:  content,
	}
	if err := db.DB.Create(msg).Error; err != nil {
		log.Printf("Error saving message: %v", err)
		return nil
	}
	return msg
}

func toggleReaction(messageID, userID uint, emoji string) int64 {
	var existing models.Reaction
	err := db.DB.Where("message_id = ? AND user_id = ? AND emoji = ?", messageID, userID, emoji).First(&existing).Error
	if err == nil {
		// exists → delete (toggle off)
		db.DB.Delete(&existing)
	} else {
		// doesn't exist → create
		db.DB.Create(&models.Reaction{
			MessageID: messageID,
			UserID:    userID,
			Emoji:     emoji,
		})
	}
	var count int64
	db.DB.Model(&models.Reaction{}).Where("message_id = ? AND emoji = ?", messageID, emoji).Count(&count)
	_ = fmt.Sprintf("reaction count: %d", count)
	return count
}
