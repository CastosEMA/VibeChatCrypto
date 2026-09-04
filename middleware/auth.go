package middleware

import (
	"context"
	"net/http"
	"vibechat/db"
	"vibechat/models"

	"github.com/gorilla/sessions"
)

type contextKey string

const UserContextKey contextKey = "user"

var Store *sessions.CookieStore

func InitStore(sessionKey string) {
	Store = sessions.NewCookieStore([]byte(sessionKey))
	Store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 30, // 30 days
		HttpOnly: true,
	}
}

// GetSession returns the session for the given request.
func GetSession(r *http.Request) *sessions.Session {
	session, _ := Store.Get(r, "vibechat-session")
	return session
}

// GetCurrentUser retrieves the logged-in user from session or nil.
func GetCurrentUser(r *http.Request) *models.User {
	session := GetSession(r)
	userID, ok := session.Values["user_id"]
	if !ok {
		return nil
	}
	id, ok := userID.(uint)
	if !ok {
		return nil
	}
	var user models.User
	if err := db.DB.First(&user, id).Error; err != nil {
		return nil
	}
	return &user
}

// LoginRequired middleware redirects unauthenticated users to /accounts/login/
func LoginRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetCurrentUser(r)
		if user == nil {
			http.Redirect(w, r, "/accounts/login/?next="+r.URL.Path, http.StatusFound)
			return
		}
		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserFromContext extracts the user from context (set by LoginRequired).
func UserFromContext(r *http.Request) *models.User {
	user, _ := r.Context().Value(UserContextKey).(*models.User)
	return user
}
