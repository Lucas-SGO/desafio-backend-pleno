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

	// Cursor-based pagination when ?cursor= is present; offset otherwise.
	if cursor := c.Query("cursor"); cursor != "" {
		items, nextCursor, hasMore, err := h.repo.ListCursor(c.Request.Context(), cpfHash, limit, cursor)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if items == nil {
			items = []domain.Notification{}
		}
		c.JSON(http.StatusOK, gin.H{
			"data":        items,
			"next_cursor": nextCursor,
			"has_more":    hasMore,
			"limit":       limit,
		})
		return
	}

	// First page via cursor (no cursor param).
	items, nextCursor, hasMore, err := h.repo.ListCursor(c.Request.Context(), cpfHash, limit, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	if items == nil {
		items = []domain.Notification{}
	}
	c.JSON(http.StatusOK, gin.H{
		"data":        items,
		"next_cursor": nextCursor,
		"has_more":    hasMore,
		"limit":       limit,
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
