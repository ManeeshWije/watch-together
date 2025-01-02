package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ManeeshWije/watch-together/utils"
	"github.com/gorilla/websocket"
	"github.com/kkdai/youtube/v2"
)

var msg struct {
	Type string  `json:"type"`
	Key  *string `json:"key"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin: func(r *http.Request) bool {
		return r.Host == "localhost:8080" || r.Host == "watch-together.up.railway.app"
	},
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
	ytClient := youtube.Client{}

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

			switch msg.Type {
			case "VIDEO_KEY":
				log.Printf("Received video key: %s", *msg.Key)
				bytes, err := utils.GetObject(*s3Client, bucket, msg.Key)
				if err != nil {
					log.Println(err)
					return
				}
				// Send video as binary message
				err = ws.WriteMessage(websocket.BinaryMessage, bytes)
				if err != nil {
					log.Println(err)
					return
				}
				log.Println("Video sent to client")
			case "FETCH_VIDEO":
				if msg.Key != nil {
					videoURL := *msg.Key
					videoID, err := utils.ExtractVideoID(videoURL)
					if err != nil {
						log.Println("ERROR: Could not parse out videoID", err)
					}

					video, err := utils.GetVideoMetadata(ytClient, videoID)
					if err != nil {
						log.Println("ERROR: Could not fetch youtube video title", err)
					}

					err = utils.StreamToS3(ytClient, video, bucket, fmt.Sprintf("%s.mp4", video.Title), *s3Client, ws)
					if err != nil {
						log.Println("Error uploading video to S3:", err)
					}
				}
			case "DELETE":
				if msg.Key != nil {
					videoTitle := *msg.Key
					log.Printf("Deleting %s...", videoTitle)

					err := utils.DeleteObject(*s3Client, bucket, videoTitle, ws)
					if err != nil {
						log.Println("Error deleteing video from S3:", err)
					}
				}
			default:
				log.Printf("Unhandled message type: %s", msg.Type)
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
	http.Handle("/client/", LogMiddleware(http.StripPrefix("/client/", clientfs)))
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
