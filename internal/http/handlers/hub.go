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
	conns map[*websocket.Conn]struct{}
}

type LiveMessage struct {
	Type  string `json:"type"`
	Item  any    `json:"item,omitempty"`
	Items any    `json:"items,omitempty"`
	Time  string `json:"time"`
}

func NewLiveHub() *LiveHub {
	return &LiveHub{
		conns: make(map[*websocket.Conn]struct{}),
	}
}

func (h *LiveHub) Add(conn *websocket.Conn) {
	h.mu.Lock()
	h.conns[conn] = struct{}{}
	h.mu.Unlock()
}

func (h *LiveHub) Remove(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.conns, conn)
	h.mu.Unlock()
}

func (h *LiveHub) Broadcast(message LiveMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	payload, _ := json.Marshal(message)
	for conn := range h.conns {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = conn.Write(ctx, websocket.MessageText, payload)
		cancel()
	}
}

func (h *LiveHub) Send(conn *websocket.Conn, message LiveMessage) {
	payload, _ := json.Marshal(message)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = conn.Write(ctx, websocket.MessageText, payload)
	cancel()
}
