//go:build testhelpers
// +build testhelpers

// Sample Go WebSocket client for market data
package test

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	url := "ws://localhost:8080/ws/marketdata"
	if len(os.Args) > 1 {
		url = os.Args[1]
	}
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	// Subscribe to orderbook and trade channels
	subMsg := map[string][]string{"subscribe": {"orderbook", "trade"}}
	c.WriteJSON(subMsg)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	go func() {
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			var pretty map[string]interface{}
			if err := json.Unmarshal(message, &pretty); err == nil {
				b, _ := json.MarshalIndent(pretty, "", "  ")
				log.Printf("recv: %s", b)
			} else {
				log.Printf("recv: %s", message)
			}
		}
	}()

	for {
		select {
		case <-interrupt:
			log.Println("interrupt")
			// Clean close
			c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			time.Sleep(time.Second)
			return
		}
	}
}
