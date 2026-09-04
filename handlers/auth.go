package handlers

import (
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"vibechat/db"
	"vibechat/middleware"
	"vibechat/models"

	"golang.org/x/crypto/bcrypt"
)

var templates map[string]*template.Template

var funcMap = template.FuncMap{
	"truncate": func(s string, n int) string {
		runes := []rune(s)
		if len(runes) <= n {
			return s
		}
		return string(runes[:n]) + "..."
	},
	"isImage": func(name string) bool {
		lower := strings.ToLower(name)
		for _, ext := range []string{".jpg", ".jpeg", ".png", ".gif", ".webp"} {
			if strings.HasSuffix(lower, ext) {
				return true
			}
		}
		return false
	},
	"not": func(b bool) bool { return !b },
	"add": func(a, b int) int { return a + b },
}

// InitTemplates loads all page templates, each combined with base.html and sidebar.html.
func InitTemplates() {
	templates = make(map[string]*template.Template)

	base := "templates/base.html"
	sidebar := "templates/chat/sidebar.html"

	// Collect all page templates
	var pages []string
	err := filepath.WalkDir("templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Base(path) == "base.html" || filepath.Base(path) == "sidebar.html" {
			return nil
		}
		if strings.HasSuffix(path, ".html") {
			pages = append(pages, path)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to walk templates: %v", err)
	}

	for _, page := range pages {
		name := filepath.Base(page)
		files := []string{base, sidebar, page}
		t := template.Must(template.New("base").Funcs(funcMap).ParseFiles(files...))
		templates[name] = t
	}

	log.Printf("Loaded %d templates", len(templates))
}

type flashMsg struct {
	Tag  string
	Text string
}

func getFlashes(w http.ResponseWriter, r *http.Request) []flashMsg {
	session := middleware.GetSession(r)
	var flashes []flashMsg
	for _, raw := range session.Flashes() {
		if s, ok := raw.(string); ok {
			parts := strings.SplitN(s, ":", 2)
			if len(parts) == 2 {
				flashes = append(flashes, flashMsg{Tag: parts[0], Text: parts[1]})
			} else {
				flashes = append(flashes, flashMsg{Tag: "info", Text: s})
			}
		}
	}
	session.Save(r, w)
	return flashes
}

func addFlash(w http.ResponseWriter, r *http.Request, tag, text string) {
	session := middleware.GetSession(r)
	session.AddFlash(tag + ":" + text)
	session.Save(r, w)
}

func renderTemplate(w http.ResponseWriter, r *http.Request, name string, data map[string]interface{}) {
	if data == nil {
		data = map[string]interface{}{}
	}
	user := middleware.GetCurrentUser(r)
	data["User"] = user
	data["Flashes"] = getFlashes(w, r)

	// Sidebar: public rooms
	var publicRooms []models.Room
	db.DB.Where("room_type = ?", "public").Order("created_at desc").Find(&publicRooms)
	data["PublicRoomsNav"] = publicRooms

	// Sidebar: my rooms
	if user != nil {
		var myRooms []models.Room
		db.DB.Raw(`SELECT rooms.* FROM rooms 
			INNER JOIN room_members ON rooms.id = room_members.room_id 
			WHERE room_members.user_id = ? AND rooms.room_type != 'public'`, user.ID).Scan(&myRooms)
		data["MyRoomsNav"] = myRooms
		data["UserInitials"] = user.GetInitials()
	}

	t, ok := templates[name]
	if !ok {
		log.Printf("Template not found: %s", name)
		http.Error(w, "Template not found: "+name, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		log.Printf("Template error (%s): %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// ─── Register ─────────────────────────────────────────────────────────

func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetCurrentUser(r)
	if user != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	data := map[string]interface{}{"Errors": map[string]string{}}

	if r.Method == http.MethodPost {
		r.ParseForm()
		username := strings.TrimSpace(r.FormValue("username"))
		email := strings.TrimSpace(r.FormValue("email"))
		password1 := r.FormValue("password1")
		password2 := r.FormValue("password2")

		errs := map[string]string{}

		if username == "" {
			errs["username"] = "Введіть логін"
		} else {
			var existing models.User
			if db.DB.Where("username = ?", username).First(&existing).Error == nil {
				errs["username"] = "Цей логін вже зайнятий"
			}
		}
		if len(password1) < 8 {
			errs["password1"] = "Пароль має бути не менше 8 символів"
		}
		if password1 != password2 {
			errs["password2"] = "Паролі не співпадають"
		}

		if len(errs) == 0 {
			hash, err := bcrypt.GenerateFromPassword([]byte(password1), bcrypt.DefaultCost)
			if err != nil {
				http.Error(w, "Server error", http.StatusInternalServerError)
				return
			}
			newUser := models.User{
				Username:     username,
				Email:        email,
				PasswordHash: string(hash),
			}
			if err := db.DB.Create(&newUser).Error; err != nil {
				errs["username"] = "Помилка при реєстрації"
			} else {
				session := middleware.GetSession(r)
				session.Values["user_id"] = newUser.ID
				session.Save(r, w)
				addFlash(w, r, "success", "Ласкаво просимо, "+newUser.Username+"! 🎉")
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
		data["Errors"] = errs
		data["FormUsername"] = username
		data["FormEmail"] = email
	}

	renderTemplate(w, r, "register.html", data)
}

// ─── Login ────────────────────────────────────────────────────────────

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetCurrentUser(r)
	if user != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	data := map[string]interface{}{}

	if r.Method == http.MethodPost {
		r.ParseForm()
		username := strings.TrimSpace(r.FormValue("username"))
		password := r.FormValue("password")

		var found models.User
		err := db.DB.Where("username = ?", username).First(&found).Error
		if err != nil || bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(password)) != nil {
			data["LoginError"] = "Невірний логін або пароль"
		} else {
			session := middleware.GetSession(r)
			session.Values["user_id"] = found.ID
			session.Save(r, w)
			nextURL := r.URL.Query().Get("next")
			if nextURL == "" {
				nextURL = "/"
			}
			http.Redirect(w, r, nextURL, http.StatusFound)
			return
		}
		data["FormUsername"] = username
	}

	renderTemplate(w, r, "login.html", data)
}

// ─── Logout ───────────────────────────────────────────────────────────

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	session := middleware.GetSession(r)
	delete(session.Values, "user_id")
	session.Save(r, w)
	http.Redirect(w, r, "/accounts/login/", http.StatusFound)
}

// ─── Profile ──────────────────────────────────────────────────────────

func ProfileHandler(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r)
	data := map[string]interface{}{"ProfileUser": user, "Errors": map[string]string{}}

	if r.Method == http.MethodPost {
		r.ParseMultipartForm(10 << 20)

		firstName := strings.TrimSpace(r.FormValue("first_name"))
		lastName := strings.TrimSpace(r.FormValue("last_name"))
		bio := strings.TrimSpace(r.FormValue("bio"))

		updates := map[string]interface{}{
			"first_name": firstName,
			"last_name":  lastName,
			"bio":        bio,
		}

		file, header, err := r.FormFile("avatar")
		if err == nil {
			defer file.Close()
			avatarDir := "media/avatars"
			os.MkdirAll(avatarDir, 0755)
			filename := strings.ReplaceAll(header.Filename, " ", "_")
			avatarPath := "avatars/" + filename
			fullPath := "media/" + avatarPath
			dst, err := os.Create(fullPath)
			if err == nil {
				io.Copy(dst, file)
				dst.Close()
				updates["avatar"] = avatarPath
			}
		}

		db.DB.Model(user).Updates(updates)
		db.DB.First(user, user.ID)
		data["ProfileUser"] = user
		addFlash(w, r, "success", "Профіль оновлено!")
		http.Redirect(w, r, "/accounts/profile/", http.StatusFound)
		return
	}

	renderTemplate(w, r, "profile.html", data)
}
