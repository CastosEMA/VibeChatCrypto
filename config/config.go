package config

import "os"

type Config struct {
	Port       string
	DBPath     string
	SessionKey string
	MediaDir   string
	StaticDir  string
}

func Load() *Config {
	sessionKey := os.Getenv("SESSION_KEY")
	if sessionKey == "" {
		sessionKey = "vibechat-secret-key-change-in-production-2024"
	}
	return &Config{
		Port:       ":8000",
		DBPath:     "db.sqlite3",
		SessionKey: sessionKey,
		MediaDir:   "media",
		StaticDir:  "static",
	}
}
