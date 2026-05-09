package controllers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/ctaylor1/briefcast/internal/logging"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// EnqueuePayload represents a public type.
type EnqueuePayload struct {
	ItemIds   []string `json:"itemIds"`
	PodcastID string   `json:"podcastID"`
	TagIds    []string `json:"tagIds"`
}

// Message represents a public type.
type Message struct {
	Identifier  string          `json:"identifier"`
	MessageType string          `json:"messageType"`
	Payload     string          `json:"payload"`
	Connection  *websocket.Conn `json:"-"`
}

// Hub serializes all WebSocket state through a single goroutine (Run),
// eliminating concurrent map access.
type Hub struct {
	activePlayers  map[*websocket.Conn]string
	allConnections map[*websocket.Conn]string
	broadcast      chan Message
}

// NewHub creates a Hub ready for use.
func NewHub() *Hub {
	return &Hub{
		activePlayers:  make(map[*websocket.Conn]string),
		allConnections: make(map[*websocket.Conn]string),
		broadcast:      make(chan Message),
	}
}

// DefaultHub is the process-wide hub started in main.
var DefaultHub = NewHub()

var wsupgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return strings.EqualFold(u.Host, r.Host)
	},
}

// Handler upgrades the HTTP connection to WebSocket, reads messages,
// and forwards everything (including disconnect) through the hub channel.
func (h *Hub) Handler(c *gin.Context) {
	logger := logging.LoggerFromGin(c).Sugar().With("component", "websocket")
	conn, err := wsupgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Errorw("websocket upgrade failed", "error", err)
		return
	}
	defer func() {
		conn.Close()
		h.broadcast <- Message{MessageType: "Disconnect", Connection: conn}
	}()
	for {
		var mess Message
		if err := conn.ReadJSON(&mess); err != nil {
			break
		}
		mess.Connection = conn
		h.broadcast <- mess
	}
}

// Run processes messages in a single goroutine so the connection maps
// are never accessed concurrently.
func (h *Hub) Run() {
	logger := logging.Sugar().With("component", "websocket")
	writeMessage := func(connection *websocket.Conn, message Message) {
		if err := connection.WriteJSON(message); err != nil {
			logger.Warnw("websocket write failed", "identifier", message.Identifier, "message_type", message.MessageType, "error", err)
			delete(h.allConnections, connection)
			delete(h.activePlayers, connection)
		}
	}
	for msg := range h.broadcast {
		if msg.MessageType != "Disconnect" && msg.Connection != nil {
			h.allConnections[msg.Connection] = msg.Identifier
		}

		switch msg.MessageType {
		case "Disconnect":
			identifier := h.allConnections[msg.Connection]
			isPlayer := h.activePlayers[msg.Connection] != ""
			delete(h.allConnections, msg.Connection)
			delete(h.activePlayers, msg.Connection)
			if isPlayer {
				for connection := range h.allConnections {
					writeMessage(connection, Message{
						Identifier:  identifier,
						MessageType: "NoPlayer",
					})
				}
				logger.Infow("player removed", "identifier", identifier)
			}
		case "RegisterPlayer":
			h.activePlayers[msg.Connection] = msg.Identifier
			for connection := range h.allConnections {
				writeMessage(connection, Message{
					Identifier:  msg.Identifier,
					MessageType: "PlayerExists",
				})
			}
			logger.Infow("player registered", "identifier", msg.Identifier)
		case "PlayerRemoved":
			for connection := range h.allConnections {
				writeMessage(connection, Message{
					Identifier:  msg.Identifier,
					MessageType: "NoPlayer",
				})
			}
			logger.Infow("player removed", "identifier", msg.Identifier)
		case "Enqueue":
			var payload EnqueuePayload
			err := json.Unmarshal([]byte(msg.Payload), &payload)
			if err == nil {
				items := getItemsToPlay(payload.ItemIds, payload.PodcastID, payload.TagIds)
				var player *websocket.Conn
				for connection, id := range h.activePlayers {
					if msg.Identifier == id {
						player = connection
						break
					}
				}
				if player != nil {
					payloadStr, err := json.Marshal(items)
					if err == nil {
						writeMessage(player, Message{
							Identifier:  msg.Identifier,
							MessageType: "Enqueue",
							Payload:     string(payloadStr),
						})
					}
				}
			} else {
				logger.Errorw("enqueue payload decode failed", "identifier", msg.Identifier, "error", err)
			}
		case "Register":
			var player *websocket.Conn
			for connection, id := range h.activePlayers {
				if msg.Identifier == id {
					player = connection
					break
				}
			}

			if player == nil {
				logger.Infow("player lookup returned none", "identifier", msg.Identifier)
				writeMessage(msg.Connection, Message{
					Identifier:  msg.Identifier,
					MessageType: "NoPlayer",
				})
			} else {
				writeMessage(msg.Connection, Message{
					Identifier:  msg.Identifier,
					MessageType: "PlayerExists",
				})
			}
		}
	}
}
