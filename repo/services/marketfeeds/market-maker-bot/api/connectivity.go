package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type Connection struct {
	ExchangeName string
	WSConn       *websocket.Conn
	RESTClient   *http.Client
}

func NewConnection(exchangeName string, wsURL string) (*Connection, error) {
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	restClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	return &Connection{
		ExchangeName: exchangeName,
		WSConn:       wsConn,
		RESTClient:   restClient,
	}, nil
}

func (c *Connection) Close() error {
	if err := c.WSConn.Close(); err != nil {
		return fmt.Errorf("failed to close WebSocket connection: %w", err)
	}
	return nil
}

func (c *Connection) SendMessage(message []byte) error {
	err := c.WSConn.WriteMessage(websocket.TextMessage, message)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	return nil
}

func (c *Connection) ReceiveMessage() ([]byte, error) {
	_, message, err := c.WSConn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to receive message: %w", err)
	}
	return message, nil
}