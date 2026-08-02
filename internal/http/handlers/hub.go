package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

type LiveHub struct {
	mu    sync.RWMutex
	conns map[*websocket.Conn]*liveHubConnection
}

type liveHubConnection struct {
	conn *websocket.Conn
	send chan []byte
	done chan struct{}
}

type LiveMessage struct {
	Type  string `json:"type"`
	Item  any    `json:"item,omitempty"`
	Items any    `json:"items,omitempty"`
	Time  string `json:"time"`
}

func NewLiveHub() *LiveHub {
	return &LiveHub{
		conns: make(map[*websocket.Conn]*liveHubConnection),
	}
}

func (h *LiveHub) Add(conn *websocket.Conn) {
	client := &liveHubConnection{
		conn: conn,
		send: make(chan []byte, 256),
		done: make(chan struct{}),
	}
	h.mu.Lock()
	if _, exists := h.conns[conn]; exists {
		h.mu.Unlock()
		return
	}
	h.conns[conn] = client
	h.mu.Unlock()
	go h.writeLoop(client)
}

func (h *LiveHub) Remove(conn *websocket.Conn) {
	h.mu.Lock()
	client := h.conns[conn]
	delete(h.conns, conn)
	h.mu.Unlock()
	if client != nil {
		close(client.done)
	}
}

func (h *LiveHub) Broadcast(message LiveMessage) {
	payload, err := json.Marshal(message)
	if err != nil {
		return
	}
	h.mu.RLock()
	for _, client := range h.conns {
		enqueueLivePayload(client, payload)
	}
	h.mu.RUnlock()
}

func (h *LiveHub) Send(conn *websocket.Conn, message LiveMessage) {
	payload, err := json.Marshal(message)
	if err != nil {
		return
	}
	h.mu.RLock()
	client := h.conns[conn]
	if client != nil {
		enqueueLivePayload(client, payload)
	}
	h.mu.RUnlock()
}

func enqueueLivePayload(client *liveHubConnection, payload []byte) {
	select {
	case client.send <- payload:
	default:
		select {
		case <-client.send:
		default:
		}
		select {
		case client.send <- payload:
		default:
		}
	}
}

func (h *LiveHub) writeLoop(client *liveHubConnection) {
	for {
		select {
		case payload := <-client.send:
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			err := client.conn.Write(ctx, websocket.MessageText, payload)
			cancel()
			if err != nil {
				_ = client.conn.CloseNow()
				return
			}
		case <-client.done:
			return
		}
	}
}
