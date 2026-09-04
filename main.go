package main

import (
	"log"
	"net/http"
	"vibechat/config"
	"vibechat/db"
	"vibechat/handlers"
	"vibechat/middleware"

	"github.com/gorilla/mux"
)

func main() {
	cfg := config.Load()

	// Init
	db.Init(cfg.DBPath)
	middleware.InitStore(cfg.SessionKey)
	handlers.InitTemplates()

	// Start WebSocket hub
	go handlers.GlobalHub.Run()

	// Router
	r := mux.NewRouter()

	// Static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	r.PathPrefix("/media/").Handler(http.StripPrefix("/media/", http.FileServer(http.Dir("media"))))

	// WebSocket
	r.HandleFunc("/ws/chat/{room_slug:[\\w-]+}/", handlers.ChatWSHandler)
	r.HandleFunc("/ws/online/", handlers.OnlineStatusWSHandler)

	// Auth routes (no middleware)
	r.HandleFunc("/accounts/register/", handlers.RegisterHandler).Methods("GET", "POST")
	r.HandleFunc("/accounts/login/", handlers.LoginHandler).Methods("GET", "POST")
	r.HandleFunc("/accounts/logout/", handlers.LogoutHandler).Methods("GET", "POST")

	// Protected routes
	protected := r.NewRoute().Subrouter()
	protected.Use(middleware.LoginRequired)

	protected.HandleFunc("/accounts/profile/", handlers.ProfileHandler).Methods("GET", "POST")
	protected.HandleFunc("/", handlers.HomeHandler).Methods("GET")
	protected.HandleFunc("/room/create/", handlers.CreateRoomHandler).Methods("GET", "POST")
	protected.HandleFunc("/room/{room_slug:[\\w-]+}/upload/", handlers.UploadFileHandler).Methods("POST")
	protected.HandleFunc("/room/{slug:[\\w-]+}/", handlers.RoomHandler).Methods("GET")
	protected.HandleFunc("/dm/{username}/", handlers.DirectMessageHandler).Methods("GET")
	protected.HandleFunc("/users/", handlers.UsersListHandler).Methods("GET")

	log.Printf("VibeChat Go server running on http://localhost%s", cfg.Port)
	log.Fatal(http.ListenAndServe(cfg.Port, r))
}
