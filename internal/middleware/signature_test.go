package middleware_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lucaseray/desafio-backend-pleno/internal/middleware"
	"github.com/stretchr/testify/assert"
)

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))
}

func TestWebhookSignature_Valid(t *testing.T) {
	router := gin.New()
	router.Use(middleware.WebhookSignature("secret"))
	router.POST("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	body := []byte(`{"chamado_id":"CH-001"}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-Signature-256", sign(body, "secret"))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestWebhookSignature_Invalid(t *testing.T) {
	router := gin.New()
	router.Use(middleware.WebhookSignature("secret"))
	router.POST("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	body := []byte(`{"chamado_id":"CH-001"}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("X-Signature-256", "sha256=deadbeef")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestWebhookSignature_Missing(t *testing.T) {
	router := gin.New()
	router.Use(middleware.WebhookSignature("secret"))
	router.POST("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	body := []byte(`{"chamado_id":"CH-001"}`)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
