package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"pulsepoll/internal/models"
	"pulsepoll/internal/store"
)

type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}
type Hub struct {
	Redis *redis.Client
	Store *store.Store
	mu    sync.RWMutex
	rooms map[string]map[*Client]bool
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func NewHub(r *redis.Client, s *store.Store) *Hub {
	return &Hub{Redis: r, Store: s, rooms: map[string]map[*Client]bool{}}
}

func (h *Hub) Run(ctx context.Context) {
	sub := h.Redis.PSubscribe(ctx, "poll.events.*")
	ch := sub.Channel()
	for msg := range ch {
		var event map[string]any
		if json.Unmarshal([]byte(msg.Payload), &event) != nil {
			continue
		}
		slug, _ := event["slug"].(string)
		h.broadcast(slug, []byte(msg.Payload))
	}
}
func (h *Hub) add(slug string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[slug] == nil {
		h.rooms[slug] = map[*Client]bool{}
	}
	h.rooms[slug][c] = true
}
func (h *Hub) remove(slug string, c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms[slug], c)
	if len(h.rooms[slug]) == 0 {
		delete(h.rooms, slug)
	}
}
func (h *Hub) broadcast(slug string, b []byte) {
	h.mu.RLock()
	clients := make([]*Client, 0, len(h.rooms[slug]))
	for c := range h.rooms[slug] {
		clients = append(clients, c)
	}
	h.mu.RUnlock()
	for _, c := range clients {
		c.mu.Lock()
		err := c.conn.WriteMessage(websocket.TextMessage, b)
		c.mu.Unlock()
		if err != nil {
			h.remove(slug, c)
			_ = c.conn.Close()
		}
	}
}
