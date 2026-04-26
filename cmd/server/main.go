package main

import (
	"log"

	"github.com/lucaseray/desafio-backend-pleno/internal/config"
)

func main() {
	cfg := config.Load()
	log.Printf("server starting on port %s", cfg.Port)
}
