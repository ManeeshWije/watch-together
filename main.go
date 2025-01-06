package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

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
		return origin == "http://localhost:8080" || origin == "https://watch-together.up.railway.app"
	},
}

func wsEndpoint(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin(r)

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer ws.Close()

	// Retrieve the session ID from the request
	cookie, err := r.Cookie("auth")
	if err != nil {
		log.Println("No auth cookie found:", err)
		return
	}

	sessionID, err := uuid.Parse(cookie.Value)
	if err != nil {
		log.Println("Invalid auth cookie value:", err)
		return
	}

	// Fetch user
	user, err := db.GetUserBySession(dbConn, sessionID)
	if err != nil {
		log.Println("Error fetching user by session", err)
		return
	}

	// Add the WebSocket connection to the manager
	websocketmanager.AddConnection(sessionID, user.Username, ws)
	defer websocketmanager.RemoveConnection(sessionID)
	log.Println("Client Connected")

	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			log.Println(err)
			websocketmanager.RemoveConnection(sessionID)
			break
		}
		if string(message) == "PLAY" || string(message) == "PAUSE" || strings.Contains(string(message), "TIMESTAMP") {
			websocketmanager.BroadcastMessage(ws, string(message))
		} else {
			err = json.Unmarshal(message, &msg)
			if err != nil {
				log.Println("Error unmarshaling message:", err)
				continue
			}

			switch msg.Type {
			default:
				log.Printf("Unhandled message type: %s", msg.Type)
			}
		}
	}
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now().UTC()
		log.Printf("Received request: Method: %s, URI: %s, RemoteAddr: %s", r.Method, r.RequestURI, r.RemoteAddr)
		next.ServeHTTP(w, r)
		log.Printf("Request processed in %s\n", time.Since(start))
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

func setupRoutes(dbConn *sql.DB, rateLimiter *ratelimiter.RateLimiter) {
	clientfs := http.FileServer(http.Dir("client"))
	http.Handle("/client/", logMiddleware(rateLimiter.Middleware(http.StripPrefix("/client/", clientfs))))
	http.Handle("/", logMiddleware(rateLimiter.Middleware(dbHandler(dbConn, utils.IndexHandler))))
	http.Handle("/logout", logMiddleware(rateLimiter.Middleware(dbHandler(dbConn, utils.LogoutHandler))))

	http.Handle("/ws", logMiddleware(authMiddleware(dbConn, rateLimiter.Middleware(dbHandler(dbConn, wsEndpoint)))))
	http.Handle("/videos", logMiddleware(authMiddleware(dbConn, rateLimiter.Middleware(dbHandler(dbConn, utils.ListVideosHandler)))))
	http.Handle("/list-users", logMiddleware(authMiddleware(dbConn, rateLimiter.Middleware(http.HandlerFunc(utils.ListUsersHandler)))))
	http.Handle("/get-video", logMiddleware(authMiddleware(dbConn, rateLimiter.Middleware(dbHandler(dbConn, utils.GetVideoHandler)))))
	http.Handle("/add-video", logMiddleware(authMiddleware(dbConn, rateLimiter.Middleware(dbHandler(dbConn, utils.AddVideoHandler)))))
	http.Handle("/delete-video", logMiddleware(authMiddleware(dbConn, rateLimiter.Middleware(dbHandler(dbConn, utils.DeleteVideoHandler)))))

	http.Handle("/auth/google/login", logMiddleware(rateLimiter.Middleware(http.HandlerFunc(utils.OauthGoogleLogin))))
	http.Handle("/auth/google/callback", logMiddleware(rateLimiter.Middleware(dbHandler(dbConn, utils.OauthGoogleCallback))))
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
	}
	dbConn := db.Connect()
	defer dbConn.Close()
	db.Migrate()
	rateLimiter := ratelimiter.NewRateLimiter(5, 10)
	utils.InitOAuthConfig()
	setupRoutes(dbConn, rateLimiter)
	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
