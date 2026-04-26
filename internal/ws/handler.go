package ws

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/lucaseray/desafio-backend-pleno/internal/middleware"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type Handler struct {
	hub *Hub
}

func NewHandler(hub *Hub) *Handler {
	return &Handler{hub: hub}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("", h.handleWS)
}

func (h *Handler) handleWS(c *gin.Context) {
	cpfHash := c.GetString(middleware.CPFHashKey)
	if cpfHash == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := NewClient(h.hub, conn, cpfHash)
	h.hub.Register(client)

	go client.writePump()
	go client.readPump()
}
