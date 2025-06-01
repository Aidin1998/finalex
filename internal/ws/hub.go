// Package ws provides a high-performance, sharded WebSocket Hub with replay buffers and lifecycle management.
package ws

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// EnhancedMessage extends Message with additional metadata for enhanced clients
type EnhancedMessage struct {
	Message
	ClientID    string            `json:"client_id,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Priority    int               `json:"priority,omitempty"`
	Retries     int               `json:"retries,omitempty"`
	ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

// Message wraps a WebSocket payload with sequencing for replay.
type Message struct {
	Topic string `json:"topic"`
	Seq   uint64 `json:"seq"`
	Data  []byte `json:"data"`
}

// ringBuffer holds the last N messages for a topic.
type ringBuffer struct {
	mu    sync.RWMutex
	buf   []Message
	size  int
	start int
	count int
}

func newRingBuffer(size int) *ringBuffer {
	return &ringBuffer{buf: make([]Message, size), size: size}
}

// add appends a message, overwriting old entries when full.
func (r *ringBuffer) add(msg Message) {
	r.mu.Lock()
	defer r.mu.Unlock()
	idx := (r.start + r.count) % r.size
	if r.count == r.size {
		r.start = (r.start + 1) % r.size
		r.count--
	}
	r.buf[idx] = msg
	r.count++
}

// getSince returns messages with Seq > since.
func (r *ringBuffer) getSince(since uint64) []Message {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Message
	for i := 0; i < r.count; i++ {
		msg := r.buf[(r.start+i)%r.size]
		if msg.Seq > since {
			out = append(out, msg)
		}
	}
	return out
}

// Client represents a single WebSocket connection.
type Client struct {
	id            string // unique client identifier
	conn          *websocket.Conn
	send          chan Message
	subscriptions map[string]uint64 // topic->latest seq acked
	hub           *Hub
}

// Hub manages all WebSocket clients, sharded for concurrency.
type Hub struct {
	shards     []*hubShard
	shardCount uint32

	register   chan *Client
	unregister chan *Client
	broadcast  chan Message

	buffers map[string]*ringBuffer
	bufMu   sync.Mutex
	seqMu   sync.Mutex
	nextSeq uint64

	upgrader websocket.Upgrader
}

type hubShard struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

// NewHub creates a Hub with given shard count and replay buffer size per topic.
func NewHub(shardCount int, replaySize int) *Hub {
	h := &Hub{
		shards:     make([]*hubShard, shardCount),
		shardCount: uint32(shardCount),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan Message, 1024),
		buffers:    make(map[string]*ringBuffer),
		nextSeq:    1,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}
	for i := range h.shards {
		h.shards[i] = &hubShard{clients: make(map[*Client]struct{})}
	}
	go h.run()
	return h
}

// run handles registration, unregistration, and broadcasting.
func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			sh := h.shardFor(client.id)
			sh.mu.Lock()
			sh.clients[client] = struct{}{}
			sh.mu.Unlock()
		case client := <-h.unregister:
			sh := h.shardFor(client.id)
			sh.mu.Lock()
			delete(sh.clients, client)
			sh.mu.Unlock()
			close(client.send)
		case msg := <-h.broadcast:
			// store in replay buffer
			h.bufMu.Lock()
			buf, ok := h.buffers[msg.Topic]
			if !ok {
				buf = newRingBuffer(1000)
				h.buffers[msg.Topic] = buf
			}
			buf.add(msg)
			h.bufMu.Unlock()
			// fan-out to all shards
			for _, sh := range h.shards {
				sh.mu.RLock()
				for c := range sh.clients {
					if _, sub := c.subscriptions[msg.Topic]; sub {
						select {
						case c.send <- msg:
						default:
							// drop slow client
						}
					}
				}
				sh.mu.RUnlock()
			}
		}
	}
}

func (h *Hub) shardFor(key string) *hubShard {
	hasher := fnv.New32a()
	hasher.Write([]byte(key))
	idx := hasher.Sum32() % h.shardCount
	return h.shards[idx]
}

// ServeWS upgrades HTTP to WS and registers the client under given clientID.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, clientID string) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &Client{
		id:            clientID,
		conn:          conn,
		send:          make(chan Message, 256),
		subscriptions: make(map[string]uint64),
		hub:           h,
	}
	h.register <- c
	go c.writePump()
	go c.readPump()
}

// Broadcast publishes a message to a topic for all subscribed clients.
func (h *Hub) Broadcast(topic string, data []byte) {
	h.seqMu.Lock()
	seq := h.nextSeq
	h.nextSeq++
	h.seqMu.Unlock()
	h.broadcast <- Message{Topic: topic, Seq: seq, Data: data}
}

// Replay returns buffered messages for topic since the given sequence.
func (h *Hub) Replay(topic string, since uint64) []Message {
	h.bufMu.Lock()
	defer h.bufMu.Unlock()
	if buf, ok := h.buffers[topic]; ok {
		return buf.getSince(since)
	}
	return nil
}

// readPump handles incoming control frames and subscription requests.
func (c *Client) readPump() {
	defer func() { c.hub.unregister <- c; c.conn.Close() }()
	c.conn.SetReadLimit(512)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		// interpret msg as JSON subscription: {"subscribe":["topic1"]}
		var req map[string][]string
		if err := json.Unmarshal(msg, &req); err == nil {
			if subs, ok := req["subscribe"]; ok {
				for _, topic := range subs {
					c.subscriptions[topic] = 0 // start from beginning
					// send replay
					for _, m := range c.hub.Replay(topic, 0) {
						c.send <- m
					}
				}
			}
			if unsubs, ok := req["unsubscribe"]; ok {
				for _, topic := range unsubs {
					delete(c.subscriptions, topic)
				}
			}
		}
	}
}

// writePump sends messages and heartbeats to the client.
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() { ticker.Stop(); c.conn.Close() }()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg.Data); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
