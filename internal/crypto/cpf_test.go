package crypto_test

import (
	"testing"

	"github.com/lucaseray/desafio-backend-pleno/internal/crypto"
	"github.com/stretchr/testify/assert"
)

func TestCPFHash_Deterministic(t *testing.T) {
	h1 := crypto.CPFHash("12345678901", "secret")
	h2 := crypto.CPFHash("12345678901", "secret")
	assert.Equal(t, h1, h2)
}

func TestCPFHash_DifferentCPF(t *testing.T) {
	h1 := crypto.CPFHash("12345678901", "secret")
	h2 := crypto.CPFHash("10987654321", "secret")
	assert.NotEqual(t, h1, h2)
}

func TestCPFHash_DifferentSecret(t *testing.T) {
	h1 := crypto.CPFHash("12345678901", "secret-a")
	h2 := crypto.CPFHash("12345678901", "secret-b")
	assert.NotEqual(t, h1, h2)
}

func TestCPFHash_NotEmpty(t *testing.T) {
	h := crypto.CPFHash("12345678901", "secret")
	assert.NotEmpty(t, h)
}
