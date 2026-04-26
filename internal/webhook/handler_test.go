package webhook_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lucaseray/desafio-backend-pleno/internal/domain"
	"github.com/lucaseray/desafio-backend-pleno/internal/middleware"
	"github.com/lucaseray/desafio-backend-pleno/internal/notification"
	"github.com/lucaseray/desafio-backend-pleno/internal/webhook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeRepo implements notification.Repository for handler tests without a real DB.
type fakeRepo struct {
	created []domain.Notification
	err     error
}

func (f *fakeRepo) CreateFromEvent(_ context.Context, _, _ string, p domain.WebhookPayload) (*domain.Notification, error) {
	if f.err != nil {
		return nil, f.err
	}
	n := domain.Notification{ID: "test-id", ChamadoID: p.ChamadoID, Titulo: p.Titulo}
	f.created = append(f.created, n)
	return &n, nil
}
func (f *fakeRepo) List(_ context.Context, _ string, _, _ int) ([]domain.Notification, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) MarkRead(_ context.Context, _, _ string) error  { return nil }
func (f *fakeRepo) UnreadCount(_ context.Context, _ string) (int, error) { return 0, nil }

func buildRouter(repo notification.Repository, repoErr error) *gin.Engine {
	if repo == nil {
		repo = &fakeRepo{err: repoErr}
	}
	svc := notification.NewService(repo, nil, nil) // nil redis+dlq: best-effort
	h := webhook.NewHandler(svc, "cpf-secret")

	router := gin.New()
	g := router.Group("/webhook", middleware.WebhookSignature("webhook-secret"))
	h.Register(g)
	return router
}

func signedRequest(t *testing.T, payload []byte) *http.Request {
	t.Helper()
	mac := hmac.New(sha256.New, []byte("webhook-secret"))
	mac.Write(payload)
	sig := fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))

	req := httptest.NewRequest(http.MethodPost, "/webhook/events", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature-256", sig)
	return req
}

func validPayload() []byte {
	p := map[string]any{
		"chamado_id":      "CH-001",
		"tipo":            "status_change",
		"cpf":             "12345678901",
		"status_anterior": "aberto",
		"status_novo":     "em_analise",
		"titulo":          "Buraco",
		"descricao":       "Rua X",
		"timestamp":       "2024-11-15T14:30:00Z",
	}
	b, _ := json.Marshal(p)
	return b
}

func TestWebhookHandler_ValidEvent(t *testing.T) {
	w := httptest.NewRecorder()
	buildRouter(nil, nil).ServeHTTP(w, signedRequest(t, validPayload()))
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestWebhookHandler_InvalidSignature(t *testing.T) {
	payload := validPayload()
	req := httptest.NewRequest(http.MethodPost, "/webhook/events", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature-256", "sha256=badhash")

	w := httptest.NewRecorder()
	buildRouter(nil, nil).ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestWebhookHandler_DuplicateEvent(t *testing.T) {
	repo := &fakeRepo{err: notification.ErrDuplicateEvent}
	w := httptest.NewRecorder()
	buildRouter(repo, nil).ServeHTTP(w, signedRequest(t, validPayload()))

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	assert.Equal(t, "duplicate", body["status"])
}

func TestWebhookHandler_MissingFields(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{"tipo": "status_change"})

	// sign with missing required fields
	mac := hmac.New(sha256.New, []byte("webhook-secret"))
	mac.Write(payload)
	sig := fmt.Sprintf("sha256=%s", hex.EncodeToString(mac.Sum(nil)))

	req := httptest.NewRequest(http.MethodPost, "/webhook/events", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature-256", sig)

	w := httptest.NewRecorder()
	buildRouter(nil, nil).ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
