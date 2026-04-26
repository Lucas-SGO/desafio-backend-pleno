package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/lucaseray/desafio-backend-pleno/internal/crypto"
)

const CPFHashKey = "cpf_hash"

// BearerJWT extracts the JWT from Authorization header or ?token= query param,
// parses preferred_username, hashes the CPF, and stores it in the Gin context.
// When jwtSecret is empty, signature verification is skipped (dev/test mode).
func BearerJWT(jwtSecret, cpfHMACSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr := extractToken(c)
		if tokenStr == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}

		var claims jwt.MapClaims
		var err error

		if jwtSecret == "" {
			claims, err = parseUnsafe(tokenStr)
		} else {
			claims, err = parseSigned(tokenStr, jwtSecret)
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		cpf, ok := claims["preferred_username"].(string)
		if !ok || cpf == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing preferred_username claim"})
			return
		}

		c.Set(CPFHashKey, crypto.CPFHash(cpf, cpfHMACSecret))
		c.Next()
	}
}

func extractToken(c *gin.Context) string {
	if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return c.Query("token")
}

func parseSigned(tokenStr, secret string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})
	return claims, err
}

func parseUnsafe(tokenStr string) (jwt.MapClaims, error) {
	p := jwt.NewParser()
	claims := jwt.MapClaims{}
	_, _, err := p.ParseUnverified(tokenStr, claims)
	return claims, err
}
