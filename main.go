package main

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/ManeeshWije/watch-together/utils"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

var clients = make([]*websocket.Conn, 0)
var clientsMutex = &sync.Mutex{}

func broadcastMessage(message string) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()
	for _, client := range clients {
		err := client.WriteMessage(websocket.TextMessage, []byte(message))
		if err != nil {
			log.Printf("Error broadcasting message to client: %v", err)
			client.Close()
			removeClient(client)
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

	// Read video file
	bytes, err := utils.FetchVideo()
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

	for {
		_, message, err := ws.ReadMessage()
		if err != nil {
			log.Println(err)
			removeClient(ws)
			break
		}
		if string(message) == "PLAY" || string(message) == "PAUSE" {
			broadcastMessage(string(message))
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
	http.Handle("/ws", LogMiddleware(http.HandlerFunc(wsEndpoint)))
}

func main() {
	utils.Init()
	setupRoutes()
	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
