package ws

import (
	"context"
	"log"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Hub maintains connected WebSocket clients and dispatches Redis pub/sub messages.
// One Redis PSUBSCRIBE connection serves all clients regardless of count.
type Hub struct {
	clients    map[string]map[*Client]bool // cpfHash → set of clients
	register   chan *Client
	unregister chan *Client
	rdb        *redis.Client
}

func NewHub(rdb *redis.Client) *Hub {
	return &Hub{
		clients:    make(map[string]map[*Client]bool),
		register:   make(chan *Client, 16),
		unregister: make(chan *Client, 16),
		rdb:        rdb,
	}
}

func (h *Hub) Register(c *Client) {
	h.register <- c
}

func (h *Hub) Unregister(c *Client) {
	h.unregister <- c
}

// Run starts the hub event loop and the Redis subscriber goroutine.
// Call it in a dedicated goroutine: go hub.Run(ctx).
func (h *Hub) Run(ctx context.Context) {
	go h.subscribeRedis(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case c := <-h.register:
			if h.clients[c.cpfHash] == nil {
				h.clients[c.cpfHash] = make(map[*Client]bool)
			}
			h.clients[c.cpfHash][c] = true
		case c := <-h.unregister:
			if bucket, ok := h.clients[c.cpfHash]; ok {
				delete(bucket, c)
				if len(bucket) == 0 {
					delete(h.clients, c.cpfHash)
				}
			}
			close(c.send)
		}
	}
}

func (h *Hub) subscribeRedis(ctx context.Context) {
	ps := h.rdb.PSubscribe(ctx, "notifications:*")
	defer ps.Close()

	ch := ps.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			// channel format: "notifications:{cpfHash}"
			parts := strings.SplitN(msg.Channel, ":", 2)
			if len(parts) != 2 {
				continue
			}
			cpfHash := parts[1]
			h.broadcast(cpfHash, []byte(msg.Payload))
		}
	}
}

func (h *Hub) broadcast(cpfHash string, msg []byte) {
	for c := range h.clients[cpfHash] {
		select {
		case c.send <- msg:
		default:
			log.Printf("ws: client %s send buffer full, dropping message", cpfHash[:8])
		}
	}
}
