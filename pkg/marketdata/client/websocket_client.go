// websocket_client.go: WebSocket client SDK for market data distribution with auto-reconnect
package client

import (
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/Aidin1998/pincex_unified/internal/marketdata/distribution"
	"github.com/gorilla/websocket"
)

// WSClient is a WebSocket client with auto-reconnect
type WSClient struct {
	url               string
	reconnectInterval time.Duration
	connMu            sync.RWMutex
	conn              *websocket.Conn
	subscriptions     map[string]bool
	subsMu            sync.Mutex
	recvCh            chan distribution.Update
	quitCh            chan struct{}
}

// NewWSClient creates a new WSClient connecting to the given URL
func NewWSClient(rawURL string, reconnectInterval time.Duration) *WSClient {
	c := &WSClient{
		url:               rawURL,
		reconnectInterval: reconnectInterval,
		subscriptions:     make(map[string]bool),
		recvCh:            make(chan distribution.Update, 1000),
		quitCh:            make(chan struct{}),
	}
	go c.run()
	return c
}

// run manages connection and reconnection
func (c *WSClient) run() {
	for {
		select {
		case <-c.quitCh:
			return
		default:
			// attempt connect
			u, _ := url.Parse(c.url)
			conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				log.Printf("WSClient dial error: %v, retrying in %v", err, c.reconnectInterval)
				time.Sleep(c.reconnectInterval)
				continue
			}
			c.connMu.Lock()
			c.conn = conn
			c.connMu.Unlock()
			// resubscribe
			c.subsMu.Lock()
			for sym := range c.subscriptions {
				conn.WriteJSON(map[string]string{"symbol": sym})
			}
			c.subsMu.Unlock()
			// read loop
			for {
				var upd distribution.Update
				err := conn.ReadJSON(&upd)
				if err != nil {
					conn.Close()
					break
				}
				select {
				case c.recvCh <- upd:
				default:
				}
			}
		}
	}
}

// Subscribe subscribes to a symbol
func (c *WSClient) Subscribe(symbol string) error {
	c.subsMu.Lock()
	c.subscriptions[symbol] = true
	c.subsMu.Unlock()
	// send on existing connection
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	if c.conn != nil {
		return c.conn.WriteJSON(map[string]string{"symbol": symbol})
	}
	return nil
}

// Next returns the next update (blocking)
func (c *WSClient) Next() (distribution.Update, bool) {
	upd, ok := <-c.recvCh
	return upd, ok
}

// Close shuts down the client
func (c *WSClient) Close() {
	close(c.quitCh)
	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.connMu.Unlock()
}
