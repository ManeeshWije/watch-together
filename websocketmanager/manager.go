package websocketmanager

import (
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type ConnectionMetadata struct {
	SessionID uuid.UUID
	Username  string
	Conn      *websocket.Conn
}

var (
	connections      = make(map[uuid.UUID]*ConnectionMetadata)
	connectionsMutex = &sync.Mutex{}
)

func AddConnection(sessionID uuid.UUID, username string, conn *websocket.Conn) {
	connectionsMutex.Lock()
	defer connectionsMutex.Unlock()
	connections[sessionID] = &ConnectionMetadata{
		SessionID: sessionID,
		Username:  username,
		Conn:      conn,
	}
	log.Printf("Added connection for session %v (username: %s)", sessionID, username)
}

func RemoveConnection(sessionID uuid.UUID) {
	connectionsMutex.Lock()
	defer connectionsMutex.Unlock()
	if metadata, exists := connections[sessionID]; exists {
		metadata.Conn.Close()
		delete(connections, sessionID)
		log.Printf("WebSocket connection for session %v (username: %s) closed and removed", sessionID, metadata.Username)
	}
}

func GetConnection(sessionID uuid.UUID) *websocket.Conn {
	connectionsMutex.Lock()
	defer connectionsMutex.Unlock()
	if metadata, exists := connections[sessionID]; exists {
		return metadata.Conn
	}
	return nil
}

func GetConnectionByUsername(username string) *websocket.Conn {
	connectionsMutex.Lock()
	defer connectionsMutex.Unlock()
	for _, metadata := range connections {
		if metadata.Username == username {
			return metadata.Conn
		}
	}
	return nil
}

func ListConnections() []*ConnectionMetadata {
	connectionsMutex.Lock()
	defer connectionsMutex.Unlock()
	allConnections := make([]*ConnectionMetadata, 0, len(connections))
	for _, metadata := range connections {
		allConnections = append(allConnections, metadata)
	}
	return allConnections
}
