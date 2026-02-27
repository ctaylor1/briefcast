package controllers

import (
	"encoding/json"

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

var wsupgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var activePlayers = make(map[*websocket.Conn]string)
var allConnections = make(map[*websocket.Conn]string)

var broadcast = make(chan Message) // broadcast channel

// Message represents a public type.
type Message struct {
	Identifier  string          `json:"identifier"`
	MessageType string          `json:"messageType"`
	Payload     string          `json:"payload"`
	Connection  *websocket.Conn `json:"-"`
}

// Wshandler handles the corresponding operation.
func Wshandler(c *gin.Context) {
	logger := logging.LoggerFromGin(c).Sugar().With("component", "websocket")
	conn, err := wsupgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Errorw("websocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()
	for {
		var mess Message
		err := conn.ReadJSON(&mess)
		if err != nil {
			isPlayer := activePlayers[conn] != ""
			if isPlayer {
				delete(activePlayers, conn)
				broadcast <- Message{
					MessageType: "PlayerRemoved",
					Identifier:  mess.Identifier,
				}
			}
			delete(allConnections, conn)
			break
		}
		mess.Connection = conn
		allConnections[conn] = mess.Identifier
		broadcast <- mess
	}
}

// HandleWebsocketMessages handles the corresponding operation.
func HandleWebsocketMessages() {
	logger := logging.Sugar().With("component", "websocket")
	writeMessage := func(connection *websocket.Conn, message Message) {
		if err := connection.WriteJSON(message); err != nil {
			logger.Warnw("websocket write failed", "identifier", message.Identifier, "message_type", message.MessageType, "error", err)
			delete(allConnections, connection)
			delete(activePlayers, connection)
		}
	}
	for {
		msg := <-broadcast

		switch msg.MessageType {
		case "RegisterPlayer":
			activePlayers[msg.Connection] = msg.Identifier
			for connection := range allConnections {
				writeMessage(connection, Message{
					Identifier:  msg.Identifier,
					MessageType: "PlayerExists",
				})
			}
			logger.Infow("player registered", "identifier", msg.Identifier)
		case "PlayerRemoved":
			for connection := range allConnections {
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
				for connection, id := range activePlayers {

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
			for connection, id := range activePlayers {

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
