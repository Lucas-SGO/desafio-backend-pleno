package notification

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lucaseray/desafio-backend-pleno/internal/domain"
	"github.com/lucaseray/desafio-backend-pleno/internal/middleware"
)

type Handler struct {
	repo Repository
}

func NewHandler(repo Repository) *Handler {
	return &Handler{repo: repo}
}

// Register wires routes onto rg. Static path must come before parametric.
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/unread-count", h.unreadCount)
	rg.GET("", h.list)
	rg.PATCH("/:id/read", h.markRead)
}

func (h *Handler) list(c *gin.Context) {
	cpfHash := c.GetString(middleware.CPFHashKey)

	limit := clampInt(c.DefaultQuery("limit", "20"), 1, 100)
	page := clampInt(c.DefaultQuery("page", "1"), 1, 1<<31-1)
	offset := (page - 1) * limit

	notifications, total, err := h.repo.List(c.Request.Context(), cpfHash, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if notifications == nil {
		notifications = []domain.Notification{}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  notifications,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (h *Handler) markRead(c *gin.Context) {
	cpfHash := c.GetString(middleware.CPFHashKey)
	id := c.Param("id")

	if err := h.repo.MarkRead(c.Request.Context(), id, cpfHash); err != nil {
		if errors.Is(err, ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) unreadCount(c *gin.Context) {
	cpfHash := c.GetString(middleware.CPFHashKey)

	count, err := h.repo.UnreadCount(c.Request.Context(), cpfHash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": count})
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func clampInt(s string, lo, hi int) int {
	n := atoi(s)
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
