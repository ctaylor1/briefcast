package controllers

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestWebsocketClosesOversizedMessages(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/ws", hub.Handler)
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL, _ := url.Parse(server.URL)
	wsURL.Scheme, wsURL.Path = "ws", "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	big := `{"identifier":"` + strings.Repeat("x", wsMaxMessageBytes) + `","messageType":"RegisterPlayer"}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(big)); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	// Without the limit the server accepts the message and answers PlayerExists.
	if _, payload, err := conn.ReadMessage(); err == nil {
		t.Fatalf("expected the server to close the connection, got a message: %.80s", payload)
	}
}
