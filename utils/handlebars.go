package utils

import (
	"database/sql"
	"fmt"
	"log/slog"
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
	"github.com/gorilla/websocket"
	"github.com/kkdai/youtube/v2"
)

var once sync.Once

// registerPartials registers Handlebars partial templates from the views/partials directory.
func registerPartials() {
	partials, err := os.ReadDir("views/partials")
	if err != nil {
		slog.Error("Error reading partials directory", "error", err)
		return
	}

	for _, partial := range partials {
		if !partial.IsDir() {
			partialName := partial.Name()
			partialContent, _ := os.ReadFile(filepath.Join("views/partials", partialName))
			partialName = partialName[:len(partialName)-len(filepath.Ext(partialName))] // Strip extension
			raymond.RegisterPartial(partialName, string(partialContent))
			slog.Debug("Registered partial", "name", partialName)
		}
	}
}

// RenderTemplate renders a Handlebars template with the provided data.
func RenderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	once.Do(registerPartials) // Register partials only once

	templatePath := filepath.Join("views", tmpl)
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		slog.Error("Template not found", "path", templatePath, "error", err)
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	tpl := raymond.MustParse(string(templateContent))
	result, err := tpl.Exec(data)
	if err != nil {
		slog.Error("Error rendering template", "template", tmpl, "error", err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}

	w.Write([]byte(result))
	slog.Debug("Template rendered successfully", "template", tmpl)
}

// IndexHandler serves the index page or redirects authenticated users to the videos page.
func IndexHandler(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	isAuthenticated, _ := VerifySession(dbConn, r)
	if isAuthenticated {
		slog.Debug("User authenticated, redirecting to /videos")
		http.Redirect(w, r, "/videos", http.StatusSeeOther)
	} else {
		slog.Debug("Rendering login page")
		RenderTemplate(w, "index.hbs", nil)
	}
}

// LogoutHandler handles user logout, session deletion, and cookie clearing.
func LogoutHandler(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("auth")
	if err != nil {
		slog.Warn("No auth cookie found", "error", err)
		http.Error(w, "No auth cookie found", http.StatusUnauthorized)
		return
	}

	sessionID, err := uuid.Parse(cookie.Value)
	if err != nil {
		slog.Warn("Invalid auth cookie value", "value", cookie.Value, "error", err)
		http.Error(w, "Invalid auth cookie value", http.StatusUnauthorized)
		return
	}

	websocketmanager.RemoveConnection(sessionID)
	if err := db.DeleteUserSessionByToken(dbConn, sessionID); err != nil {
		slog.Error("Error deleting user session", "sessionID", sessionID, "error", err)
		http.Error(w, "Error logging out", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    "",
		Expires:  time.Now().UTC().Add(-1 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	slog.Info("User logged out successfully", "sessionID", sessionID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// ListVideosHandler renders the videos page with the list of videos from the database.
func ListVideosHandler(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	isAuthenticated, _ := VerifySession(dbConn, r)
	if !isAuthenticated {
		slog.Warn("Unauthorized access attempt to list videos")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	videos, err := db.ListVideos(dbConn)
	if err != nil {
		slog.Error("Failed to list videos", "error", err)
		http.Error(w, "Failed to list videos", http.StatusInternalServerError)
		return
	}

	RenderTemplate(w, "index.hbs", map[string]interface{}{
		"Authenticated": isAuthenticated,
		"videos":        videos,
	})
	slog.Debug("Videos listed successfully")
}

// ListUsersHandler retrieves and sends a list of connected users.
func ListUsersHandler(w http.ResponseWriter, r *http.Request) {
	conns := websocketmanager.ListConnections()

	usernames := make([]string, 0, len(conns))
	for _, conn := range conns {
		usernames = append(usernames, conn.Username)
	}

	usernamesString := strings.Join(usernames, ", ")
	if _, err := w.Write([]byte(usernamesString)); err != nil {
		slog.Error("Failed to send users list", "error", err)
		http.Error(w, "Failed to send users list", http.StatusInternalServerError)
		return
	}

	slog.Debug("Users list sent successfully", "users", usernames)
}

// AddVideoHandler handles adding a new video to the database and uploads it to S3.
func AddVideoHandler(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	isAuthenticated, session := VerifySession(dbConn, r)
	if !isAuthenticated {
		slog.Warn("Unauthorized attempt to add video")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	canUpload, err := db.IncrementUploads(dbConn, session.UserUUID)
	if !canUpload {
		slog.Warn("Upload limit exceeded", "userUUID", session.UserUUID)
		http.Error(w, "Upload limit exceeded", http.StatusForbidden)
		return
	}

	ytClient := youtube.Client{}
	ws := websocketmanager.GetConnection(session.UUID)

	if err := r.ParseForm(); err != nil {
		slog.Error("Failed to parse form", "error", err)
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	videoURL := r.FormValue("video-url")
	if videoURL == "" {
		slog.Warn("Missing video URL")
		http.Error(w, "Missing video URL", http.StatusBadRequest)
		return
	}

	slog.Info("Received video URL", "videoURL", videoURL)

	videoID, err := ExtractVideoID(videoURL)
	if err != nil {
		slog.Error("Invalid video URL", "url", videoURL, "error", err)
		http.Error(w, "Invalid video URL", http.StatusBadRequest)
		return
	}

	video, err := GetVideoMetadata(ytClient, videoID)
	if err != nil {
		slog.Error("Failed to fetch video metadata", "videoID", videoID, "error", err)
		http.Error(w, "Failed to fetch video metadata", http.StatusInternalServerError)
		return
	}

	if err := db.CreateVideo(dbConn, videoURL, video.Title, time.Now().UTC()); err != nil {
		slog.Error("Database error while creating video", "video", video.Title, "error", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	s3Client, err := CreateS3Client()
	if err != nil {
		slog.Error("S3 client error", "error", err)
		http.Error(w, "S3 client error", http.StatusInternalServerError)
		return
	}

	bucket := os.Getenv("AWS_S3_BUCKET")
	if bucket == "" {
		slog.Error("S3 bucket not configured")
		http.Error(w, "S3 bucket not configured", http.StatusInternalServerError)
		return
	}

	if err := StreamToS3(ytClient, video, bucket, fmt.Sprintf("%s.mp4", video.Title), *s3Client, ws); err != nil {
		slog.Error("Error uploading to S3", "video", video.Title, "error", err)
		return
	}

	w.Header().Set("HX-Refresh", "true")
	slog.Info("Video added and uploaded successfully", "video", video.Title)
}

func DeleteVideoHandler(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	isAuthenticated, session := VerifySession(dbConn, r)
	if !isAuthenticated {
		slog.Warn("Unauthorized attempt to add video")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ws := websocketmanager.GetConnection(session.UUID)

	query := r.URL.Query()
	videoUrl := query.Get("video-url")
	if videoUrl == "" {
		slog.Warn("Missing video URL")
		http.Error(w, "Missing video url", http.StatusBadRequest)
		return
	}

	decodedUrl, err := url.QueryUnescape(videoUrl)
	if err != nil {
		slog.Warn("Invalid video url encoding")
		http.Error(w, "Invalid video url encoding", http.StatusBadRequest)
		return
	}

	err = db.DeleteVideo(dbConn, decodedUrl)
	if err != nil {
		slog.Error("Failed to delete video from db", "decodedUrl", decodedUrl, "error", err)
		http.Error(w, "Failed to delete video from database", http.StatusInternalServerError)
		return
	}

	s3Client, err := CreateS3Client()
	if err != nil {
		slog.Error("Failed to create s3 client", "error", err)
		http.Error(w, "Failed to fetch s3 client", http.StatusInternalServerError)
		return
	}
	bucket, exists := os.LookupEnv("AWS_S3_BUCKET")
	if !exists {
		slog.Error("Bucket env var not set", "exists", exists)
		http.Error(w, "Bucket env var not set", http.StatusInternalServerError)
		return
	}

	ytClient := youtube.Client{}
	video, err := ytClient.GetVideo(decodedUrl)
	if err != nil {
		slog.Error("Failed to fetch video from given url", "decodedUrl", decodedUrl, "error", err)
		http.Error(w, "Error fetching video from given URL", http.StatusInternalServerError)
		return
	}
	err = DeleteObject(*s3Client, bucket, video.Title, ws)
	if err != nil {
		slog.Error("Failed to delete video from S3", "title", video.Title, "error", err)
		http.Error(w, "Error deleting video from S3", http.StatusInternalServerError)
	}
	w.Header().Set("HX-Refresh", "true")
}

func GetVideoHandler(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	isAuthenticated, session := VerifySession(dbConn, r)
	if !isAuthenticated {
		slog.Warn("Unauthorized attempt to get video")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ws := websocketmanager.GetConnection(session.UUID)

	query := r.URL.Query()
	videoUrl := query.Get("video-url")
	videoTitle := query.Get("video-title")
	if videoUrl == "" || videoTitle == "" {
		slog.Warn("Video url or video title is missing", "url", videoUrl, "title", videoTitle)
		http.Error(w, "Missing video url or video title", http.StatusBadRequest)
		return
	}

	decodedUrl, err := url.QueryUnescape(videoUrl)
	if err != nil {
		slog.Warn("Invalid video url encoding", "url", videoUrl, "error", err)
		http.Error(w, "Invalid video url encoding", http.StatusBadRequest)
		return
	}
	decodedTitle, err := url.QueryUnescape(videoTitle)
	if err != nil {
		slog.Warn("Invalid video title encoding", "title", videoTitle, "error", err)
		http.Error(w, "Invalid video title encoding", http.StatusBadRequest)
		return
	}

	slog.Info("Received video url", "url", decodedUrl)
	slog.Info("Received video title", "title", decodedTitle)

	video, err := db.GetVideo(dbConn, decodedUrl, decodedTitle)
	if err != nil {
		slog.Error("Failed to get video from database", "error", err)
		http.Error(w, "Failed to get video from database", http.StatusInternalServerError)
		return
	}

	s3Client, err := CreateS3Client()
	if err != nil {
		slog.Error("Failed to create s3 client", "error", err)
		http.Error(w, "Failed to fetch s3 client", http.StatusInternalServerError)
		return
	}
	bucket, exists := os.LookupEnv("AWS_S3_BUCKET")
	if !exists {
		slog.Error("Bucket env var not set", "exists", exists)
		http.Error(w, "Bucket env var not set", http.StatusInternalServerError)
		return
	}

	bytes, err := GetObject(*s3Client, bucket, fmt.Sprintf("%s.mp4", video.Title))
	if err != nil {
		slog.Error("Failed to GetObject from s3", "error", err)
		return
	}

	// Send video as binary message
	err = ws.WriteMessage(websocket.BinaryMessage, bytes)
	if err != nil {
		slog.Error("Failed to write web socket binary video message", "error", err)
		return
	}
	slog.Info("Video sent to client")
}
