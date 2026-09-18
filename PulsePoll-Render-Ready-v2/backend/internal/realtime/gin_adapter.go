package realtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"pulsepoll/internal/models"
)

func (h *Hub) ServeWSGin(c *gin.Context) {
	slug := c.Query("poll")
	if slug == "" {
		c.JSON(400, gin.H{"error": "poll is required"})
		return
	}
	p, err := h.Store.GetPoll(c, slug)
	if err != nil {
		c.JSON(404, gin.H{"error": "poll not found"})
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := &Client{conn: conn}
	h.add(slug, client)
	defer h.remove(slug, client)
	defer conn.Close()
	snap, err := h.Store.Snapshot(context.Background(), p)
	if err == nil {
		client.writeJSON(map[string]any{"type": "poll.snapshot", "poll": snap})
	}
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error { conn.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			client.mu.Lock()
			_ = conn.WriteMessage(websocket.PingMessage, nil)
			client.mu.Unlock()
		}
	}()
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *Client) writeJSON(v any) {
	b, _ := json.Marshal(v)
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.WriteMessage(websocket.TextMessage, b)
}

func (h *Hub) Publish(ctx context.Context, slug string, snap models.Snapshot, optionCount int) {
	payload := map[string]any{"type": "poll.vote.updated", "slug": slug, "poll": snap, "activity": map[string]any{"secondsAgo": 0, "optionCount": optionCount}}
	b, _ := json.Marshal(payload)
	h.Redis.Publish(ctx, "poll.events."+slug, b)
}
func (h *Hub) PublishReaction(ctx context.Context, slug, optionID, emoji string) {
	b, _ := json.Marshal(map[string]any{"type": "poll.reaction", "slug": slug, "optionId": optionID, "emoji": emoji})
	h.Redis.Publish(ctx, "poll.events."+slug, b)
}

var _ *redis.Client

func (h *Hub) PublishClosed(ctx context.Context, slug string, snap models.Snapshot) {
	b, _ := json.Marshal(map[string]any{"type": "poll.closed", "slug": slug, "poll": snap})
	h.Redis.Publish(ctx, "poll.events."+slug, b)
}
