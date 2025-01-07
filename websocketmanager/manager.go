package websocketmanager

import (
	"log/slog"
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
	slog.Info("Added connection for session and username", "session", sessionID, "username", username)
}

func RemoveConnection(sessionID uuid.UUID) {
	connectionsMutex.Lock()
	defer connectionsMutex.Unlock()
	if metadata, exists := connections[sessionID]; exists {
		metadata.Conn.Close()
		delete(connections, sessionID)
		slog.Info("WebSocket connection for session username closed and removed", "session", sessionID, "username", metadata.Username)
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

func BroadcastMessage(sender *websocket.Conn, message string) {
	connections := ListConnections()

	// Broadcast to all connections except the sender
	for _, connMetadata := range connections {
		if connMetadata.Conn != sender {
			err := connMetadata.Conn.WriteMessage(websocket.TextMessage, []byte(message))
			if err != nil {
				slog.Error("Error broadcasting message to client", "error", err)
				connMetadata.Conn.Close()
				RemoveConnection(connMetadata.SessionID)
			}
		}
	}
}
