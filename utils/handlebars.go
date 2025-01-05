package utils

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ManeeshWije/watch-together/db"
	"github.com/ManeeshWije/watch-together/websocketmanager"
	"github.com/aymerick/raymond"
	"github.com/google/uuid"
	"github.com/kkdai/youtube/v2"
)

var once sync.Once

func registerPartials() {
	partials, err := os.ReadDir("views/partials")
	if err == nil {
		for _, partial := range partials {
			if !partial.IsDir() {
				partialName := partial.Name()
				partialContent, _ := os.ReadFile(filepath.Join("views/partials", partialName))
				partialName = partialName[:len(partialName)-len(filepath.Ext(partialName))] // Strip extension
				raymond.RegisterPartial(partialName, string(partialContent))
			}
		}
	}
}

func RenderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	// Register partials only once
	once.Do(registerPartials)

	templatePath := filepath.Join("views", tmpl)
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	// Parse the template
	tpl := raymond.MustParse(string(templateContent))

	// Execute the template with the provided data
	result, err := tpl.Exec(data)
	if err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}

	w.Write([]byte(result))
}

func IndexHandler(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	isAuthenticated, _ := VerifySession(dbConn, r)
	if isAuthenticated {
		// User is authenticated, redirect to /videos
		http.Redirect(w, r, "/videos", http.StatusSeeOther)
	} else {
		// If no cookie, render the login page without auth
		RenderTemplate(w, "index.hbs", nil)
	}
}

func LogoutHandler(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("auth")
	if err != nil {
		http.Error(w, "No auth cookie found", http.StatusUnauthorized)
		return
	}

	sessionID, err := uuid.Parse(cookie.Value)
	if err != nil {
		http.Error(w, "Invalid auth cookie value", http.StatusUnauthorized)
		return
	}

	// Close the WebSocket connection
	websocketmanager.RemoveConnection(sessionID)

	// Delete the session from the database
	err = db.DeleteUserSessionByToken(dbConn, sessionID)
	if err != nil {
		log.Printf("Error deleting user session: %v", err)
		http.Error(w, "Error logging out", http.StatusInternalServerError)
		return
	}

	// Clear the auth cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    "",
		Expires:  time.Now().UTC().Add(-1 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func ListVideosHandler(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	isAuthenticated, _ := VerifySession(dbConn, r)
	if !isAuthenticated {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	s3Client, err := CreateS3Client()
	if err != nil {
		http.Error(w, "Failed to fetch s3 client", http.StatusInternalServerError)
		return
	}
	bucket, exists := os.LookupEnv("AWS_S3_BUCKET")
	if !exists {
		http.Error(w, "Bucket env var not set", http.StatusInternalServerError)
		return
	}
	objects, err := ListObjects(*s3Client, bucket)
	if err != nil {
		http.Error(w, "Failed to list videos", http.StatusInternalServerError)
		return
	}

	RenderTemplate(w, "index.hbs", map[string]interface{}{
		"Authenticated": isAuthenticated,
		"objects":       objects,
	})
}

func ListUsersHandler(w http.ResponseWriter, r *http.Request) {
	conns := websocketmanager.ListConnections()

	usernames := []string{}
	for _, conn := range conns {
		usernames = append(usernames, conn.Username)
	}

	usernamesString := strings.Join(usernames, ", ")

	_, err := w.Write([]byte(usernamesString))
	if err != nil {
		http.Error(w, "Failed to send users list", http.StatusInternalServerError)
		log.Println("Error sending users list:", err)
	}
}

func AddVideoHandler(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	isAuthenticated, session := VerifySession(dbConn, r)
	if !isAuthenticated {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	canUpload, err := db.IncrementUploads(dbConn, session.UserUUID)
	if !canUpload {
		http.Error(w, "Upload limit exceeded", http.StatusForbidden)
		return
	}

	ytClient := youtube.Client{}
	ws := websocketmanager.GetConnection(session.UUID)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	videoURL := r.FormValue("video-url")
	if videoURL == "" {
		http.Error(w, "Missing video URL", http.StatusBadRequest)
		return
	}

	log.Printf("Received video URL: %s", videoURL)
	videoID, err := ExtractVideoID(videoURL)
	if err != nil {
		log.Println("ERROR: Could not parse out videoID", err)
		http.Error(w, "Could not parse out videoID", http.StatusBadRequest)
		return
	}
	video, err := GetVideoMetadata(ytClient, videoID)
	if err != nil {
		log.Println("ERROR: Could not fetch youtube video title", err)
	}

	s3Client, err := CreateS3Client()
	if err != nil {
		http.Error(w, "Failed to fetch s3 client", http.StatusInternalServerError)
		return
	}
	bucket, exists := os.LookupEnv("AWS_S3_BUCKET")
	if !exists {
		http.Error(w, "Bucket env var not set", http.StatusInternalServerError)
		return
	}

	err = StreamToS3(ytClient, video, bucket, fmt.Sprintf("%s.mp4", video.Title), *s3Client, ws)
	if err != nil {
		log.Println("Error uploading video to S3:", err)
	}

	w.Header().Set("HX-Refresh", "true")
}

func DeleteVideoHandler(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	isAuthenticated, session := VerifySession(dbConn, r)
	if !isAuthenticated {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ws := websocketmanager.GetConnection(session.UUID)

	query := r.URL.Query()
	videoTitle := query.Get("video-title")
	if videoTitle == "" {
		http.Error(w, "Missing video title", http.StatusBadRequest)
		return
	}

	decodedTitle, err := url.QueryUnescape(videoTitle)
	if err != nil {
		http.Error(w, "Invalid video title encoding", http.StatusBadRequest)
		return
	}

	log.Printf("Received video title: %s", decodedTitle)

	s3Client, err := CreateS3Client()
	if err != nil {
		http.Error(w, "Failed to fetch s3 client", http.StatusInternalServerError)
		return
	}
	bucket, exists := os.LookupEnv("AWS_S3_BUCKET")
	if !exists {
		http.Error(w, "Bucket env var not set", http.StatusInternalServerError)
		return
	}

	err = DeleteObject(*s3Client, bucket, videoTitle, ws)
	if err != nil {
		log.Println("Error deleteing video from S3:", err)
	}
	w.Header().Set("HX-Refresh", "true")
}
