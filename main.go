package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"log/slog"

	"github.com/ManeeshWije/watch-together/db"
	"github.com/ManeeshWije/watch-together/ratelimiter"
	"github.com/ManeeshWije/watch-together/utils"
	"github.com/ManeeshWije/watch-together/websocketmanager"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

var msg struct {
	Type string  `json:"type"`
	Key  *string `json:"key"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "http://localhost:8080" || origin == "https://watch.wijeproject.com"
	},
}

func wsEndpoint(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin(r)

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("Failed to upgrade connection", "error", err)
		return
	}
	defer ws.Close()

	// Retrieve the session ID from the request
	cookie, err := r.Cookie("auth")
	if err != nil {
		slog.Error("No auth cookie found", "error", err)
		return
	}

	sessionID, err := uuid.Parse(cookie.Value)
	if err != nil {
		slog.Error("Invalid auth cookie value", "error", err)
		return
	}

	// Fetch user
	user, err := db.GetUserBySession(dbConn, sessionID)
	if err != nil {
		slog.Error("Error fetching user by session", "error", err)
		return
	}

	// Add the WebSocket connection to the manager
	websocketmanager.AddConnection(sessionID, user.Username, ws)
	defer websocketmanager.RemoveConnection(sessionID)
	slog.Info("Client connected", "sessionID", sessionID, "username", user.Username)

	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			slog.Error("Error reading message", "error", err)
			websocketmanager.RemoveConnection(sessionID)
			break
		}
		if string(message) == "PLAY" || string(message) == "PAUSE" || strings.Contains(string(message), "TIMESTAMP") {
			websocketmanager.BroadcastMessage(ws, string(message))
		} else {
			err = json.Unmarshal(message, &msg)
			if err != nil {
				slog.Error("Error unmarshaling message", "error", err)
				continue
			}

			switch msg.Type {
			default:
				slog.Warn("Unhandled message type", "type", msg.Type)
			}
		}
	}
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now().UTC()
		slog.Info("Received request", "method", r.Method, "uri", r.RequestURI, "remoteAddr", r.RemoteAddr)
		next.ServeHTTP(w, r)
		slog.Info("Request processed", "duration", time.Since(start))
	})
}

func dbHandler(db *sql.DB, handler func(*sql.DB, http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handler(db, w, r)
	}
}

func authMiddleware(dbConn *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isAuthenticated, session := utils.VerifySession(dbConn, r)
		if isAuthenticated != true || session == nil {
			http.Redirect(w, r, "/", http.StatusUnauthorized)
			return
		}

		// Proceed with the next handler if the session is valid
		next.ServeHTTP(w, r)
	})
}

func dailyCleanup(dbConn *sql.DB) {
	go func() {
		for {
			now := time.Now()
			// Calculate the duration until the next midnight
			nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
			duration := time.Until(nextMidnight)

			// Wait until the next midnight
			time.Sleep(duration)

			// Run the DeleteExpiredSessions function
			if err := db.DeleteExpiredSessions(dbConn); err != nil {
				slog.Error("Error running DeleteExpiredSessions", "error", err)
			} else {
				slog.Info("Successfully ran DeleteExpiredSessions")
			}
		}
	}()
}

func setupRoutes(dbConn *sql.DB, rateLimiter *ratelimiter.RateLimiter) {
	clientfs := http.FileServer(http.Dir("client"))
	http.Handle("GET /client/", logMiddleware(rateLimiter.Middleware(http.StripPrefix("/client/", clientfs))))
	http.Handle("GET /", logMiddleware(rateLimiter.Middleware(dbHandler(dbConn, utils.IndexHandler))))
	http.Handle("POST /logout", logMiddleware(rateLimiter.Middleware(dbHandler(dbConn, utils.LogoutHandler))))

	http.Handle("GET /ws", logMiddleware(authMiddleware(dbConn, rateLimiter.Middleware(dbHandler(dbConn, wsEndpoint)))))
	http.Handle("GET /videos", logMiddleware(authMiddleware(dbConn, rateLimiter.Middleware(dbHandler(dbConn, utils.ListVideosHandler)))))
	http.Handle("GET /list-users", logMiddleware(authMiddleware(dbConn, rateLimiter.Middleware(http.HandlerFunc(utils.ListUsersHandler)))))
	http.Handle("GET /get-video", logMiddleware(authMiddleware(dbConn, rateLimiter.Middleware(dbHandler(dbConn, utils.GetVideoHandler)))))
	http.Handle("POST /add-video", logMiddleware(authMiddleware(dbConn, rateLimiter.Middleware(dbHandler(dbConn, utils.AddVideoHandler)))))
	http.Handle("POST /delete-video", logMiddleware(authMiddleware(dbConn, rateLimiter.Middleware(dbHandler(dbConn, utils.DeleteVideoHandler)))))

	http.Handle("GET /auth/google/login", logMiddleware(rateLimiter.Middleware(http.HandlerFunc(utils.OauthGoogleLogin))))
	http.Handle("GET /auth/google/callback", logMiddleware(rateLimiter.Middleware(dbHandler(dbConn, utils.OauthGoogleCallback))))
}

func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := godotenv.Load(); err != nil {
		slog.Warn("Error loading .env file", "error", err)
	}
	dbConn := db.Connect()
	defer dbConn.Close()

	db.Migrate()

	rateLimiter := ratelimiter.NewRateLimiter(5, 10)
	utils.InitOAuthConfig()
	setupRoutes(dbConn, rateLimiter)
	dailyCleanup(dbConn)
	slog.Info("Server started on :8080")
	slog.Error("Server shutdown", "error", http.ListenAndServe(":8080", nil))
}
