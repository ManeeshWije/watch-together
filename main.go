package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ManeeshWije/watch-together/utils"
	"github.com/gorilla/websocket"
)

var msg struct {
	Type string  `json:"type"`
	Key  *string `json:"key"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
}

var clients = make([]*websocket.Conn, 0)
var clientsMutex = &sync.Mutex{}

func broadcastMessage(sender *websocket.Conn, message string) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()
	for _, client := range clients {
		if client != sender {
			err := client.WriteMessage(websocket.TextMessage, []byte(message))
			if err != nil {
				log.Printf("Error broadcasting message to client: %v", err)
				client.Close()
				removeClient(client)
			}
		}
	}
}

func removeClient(conn *websocket.Conn) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()
	for i, client := range clients {
		if client == conn {
			clients = append(clients[:i], clients[i+1:]...)
			break
		}
	}
}

func wsEndpoint(w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer ws.Close()

	clientsMutex.Lock()
	clients = append(clients, ws)
	clientsMutex.Unlock()

	log.Println("Client Connected")

	s3Client, err := utils.CreateS3Client()
	if err != nil {
		log.Println(err)
		return
	}
	bucket, exists := os.LookupEnv("AWS_S3_BUCKET")
	if !exists {
		log.Println("Bucket does not exist")
		return
	}
	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			log.Println(err)
			removeClient(ws)
			break
		}
		if string(message) == "PLAY" || string(message) == "PAUSE" || strings.Contains(string(message), "TIMESTAMP") {
			broadcastMessage(ws, string(message))
		} else {
			err = json.Unmarshal(message, &msg)
			if err != nil {
				log.Println("Error unmarshaling message:", err)
				continue
			}

			if msg.Type == "VIDEO_KEY" {
				log.Printf("Received video key: %s", *msg.Key)
				chunks, err := utils.GetObject(*s3Client, bucket, msg.Key)
				if err != nil {
					log.Println(err)
					return
				}
				// Stream each chunk to the client
				for chunk := range chunks {
					err = ws.WriteMessage(websocket.BinaryMessage, chunk)
					if err != nil {
						log.Println("Error sending chunk to client: ", err)
						return
					}
				}

				log.Println("Video sent to client")
			}
		}
	}
}

func LogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("Received request: Method: %s, URI: %s, RemoteAddr: %s", r.Method, r.RequestURI, r.RemoteAddr)
		next.ServeHTTP(w, r)
		log.Printf("Request processed in %s\n", time.Since(start))
	})
}

func setupRoutes() {
	clientfs := http.FileServer(http.Dir("client"))
	distfs := http.FileServer(http.Dir("dist"))

	http.Handle("/client/", LogMiddleware(http.StripPrefix("/client/", clientfs)))
	http.Handle("/dist/", LogMiddleware(http.StripPrefix("/dist/", distfs)))

	http.Handle("/", LogMiddleware(http.HandlerFunc(utils.IndexHandler)))
	http.Handle("/submit", LogMiddleware(http.HandlerFunc(utils.SubmitHandler)))
	http.Handle("/logout", LogMiddleware(http.HandlerFunc(utils.LogoutHandler)))
	http.Handle("/ws", LogMiddleware(http.HandlerFunc(wsEndpoint)))
	http.Handle("/videos", LogMiddleware(http.HandlerFunc(utils.ListVideosHandler)))
}

func main() {
	utils.Init()
	setupRoutes()
	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
