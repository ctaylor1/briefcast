package controllers

import (
	"encoding/json"

	"github.com/akhilrex/briefcast/internal/logging"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type EnqueuePayload struct {
	ItemIds   []string `json:"itemIds"`
	PodcastId string   `json:"podcastId"`
	TagIds    []string `json:"tagIds"`
}

var wsupgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var activePlayers = make(map[*websocket.Conn]string)
var allConnections = make(map[*websocket.Conn]string)

var broadcast = make(chan Message) // broadcast channel

type Message struct {
	Identifier  string          `json:"identifier"`
	MessageType string          `json:"messageType"`
	Payload     string          `json:"payload"`
	Connection  *websocket.Conn `json:"-"`
}

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

func HandleWebsocketMessages() {
	logger := logging.Sugar().With("component", "websocket")
	for {
		msg := <-broadcast

		switch msg.MessageType {
		case "RegisterPlayer":
			activePlayers[msg.Connection] = msg.Identifier
			for connection, _ := range allConnections {
				connection.WriteJSON(Message{
					Identifier:  msg.Identifier,
					MessageType: "PlayerExists",
				})
			}
			logger.Infow("player registered", "identifier", msg.Identifier)
		case "PlayerRemoved":
			for connection, _ := range allConnections {
				connection.WriteJSON(Message{
					Identifier:  msg.Identifier,
					MessageType: "NoPlayer",
				})
			}
			logger.Infow("player removed", "identifier", msg.Identifier)
		case "Enqueue":
			var payload EnqueuePayload
			err := json.Unmarshal([]byte(msg.Payload), &payload)
			if err == nil {
				items := getItemsToPlay(payload.ItemIds, payload.PodcastId, payload.TagIds)
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
						player.WriteJSON(Message{
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
				msg.Connection.WriteJSON(Message{
					Identifier:  msg.Identifier,
					MessageType: "NoPlayer",
				})
			} else {
				msg.Connection.WriteJSON(Message{
					Identifier:  msg.Identifier,
					MessageType: "PlayerExists",
				})
			}
		}
	}
}
