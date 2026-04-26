package webhook

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lucaseray/desafio-backend-pleno/internal/crypto"
	"github.com/lucaseray/desafio-backend-pleno/internal/domain"
	"github.com/lucaseray/desafio-backend-pleno/internal/notification"
)

type Handler struct {
	svc           *notification.Service
	cpfHMACSecret string
}

func NewHandler(svc *notification.Service, cpfHMACSecret string) *Handler {
	return &Handler{svc: svc, cpfHMACSecret: cpfHMACSecret}
}

func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/events", h.handleEvent)
}

func (h *Handler) handleEvent(c *gin.Context) {
	var p domain.WebhookPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if p.ChamadoID == "" || p.CPF == "" || p.Tipo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "chamado_id, cpf, and tipo are required"})
		return
	}

	eventHash := computeEventHash(p)
	cpfHash := crypto.CPFHash(p.CPF, h.cpfHMACSecret)

	n, err := h.svc.CreateFromWebhook(c.Request.Context(), cpfHash, eventHash, p)
	if err != nil {
		if errors.Is(err, notification.ErrDuplicateEvent) {
			c.JSON(http.StatusOK, gin.H{"status": "duplicate"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "created", "id": n.ID})
}

func computeEventHash(p domain.WebhookPayload) string {
	raw := fmt.Sprintf("%s|%s|%s", p.ChamadoID, p.Tipo, p.Timestamp.UTC().Format(time.RFC3339))
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
