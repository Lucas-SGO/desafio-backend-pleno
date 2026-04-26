package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/lucaseray/desafio-backend-pleno/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func makeToken(cpf, secret string) string {
	claims := jwt.MapClaims{
		"preferred_username": cpf,
		"exp":                time.Now().Add(time.Hour).Unix(),
	}
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	return token
}

func TestBearerJWT_ValidToken(t *testing.T) {
	router := gin.New()
	router.Use(middleware.BearerJWT("test-secret", "cpf-secret"))
	router.GET("/", func(c *gin.Context) {
		cpfHash := c.GetString(middleware.CPFHashKey)
		c.String(http.StatusOK, cpfHash)
	})

	token := makeToken("12345678901", "test-secret")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.String())
}

func TestBearerJWT_MissingToken(t *testing.T) {
	router := gin.New()
	router.Use(middleware.BearerJWT("test-secret", "cpf-secret"))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBearerJWT_InvalidSignature(t *testing.T) {
	router := gin.New()
	router.Use(middleware.BearerJWT("correct-secret", "cpf-secret"))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	token := makeToken("12345678901", "wrong-secret")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBearerJWT_TokenInQueryParam(t *testing.T) {
	router := gin.New()
	router.Use(middleware.BearerJWT("", "cpf-secret")) // skip sig
	router.GET("/", func(c *gin.Context) {
		cpfHash := c.GetString(middleware.CPFHashKey)
		c.String(http.StatusOK, cpfHash)
	})

	token := makeToken("12345678901", "any-secret")
	req := httptest.NewRequest(http.MethodGet, "/?token="+token, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Body.String())
}

func TestBearerJWT_MissingClaim(t *testing.T) {
	router := gin.New()
	router.Use(middleware.BearerJWT("", "cpf-secret"))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	claims := jwt.MapClaims{"sub": "someone", "exp": time.Now().Add(time.Hour).Unix()}
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("x"))
	req := httptest.NewRequest(http.MethodGet, "/?token="+token, nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
